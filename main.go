// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2024 Matthew Penner

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/matthewpi/fiche/internal/haste"
	"github.com/matthewpi/fiche/internal/systemd"
)

var CLI struct {
	Listen   string `help:"Listen address" default:":99"`
	Hastebin string `help:"haste-server URL" placeholder:"https://ptero.co"`
	Limit    int    `help:"Maximum size per paste" default:"131072"` // 131072 = 128 * 1024 (128 KiB)
}

func main() {
	_ = kong.Parse(
		&CLI,
		kong.Name("fiche"),
	)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	h, err := haste.NewClient(CLI.Hastebin)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "failed to create hastebin client", slog.Any("err", err))
		os.Exit(1)
		return
	}

	listener, err := getListener(ctx)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "failed to start listener", slog.Any("err", err))
		os.Exit(1)
		return
	}
	defer listener.Close()

	slog.LogAttrs(ctx, slog.LevelInfo, "starting server...")
	s := NewServer(listener, h)
	go func(ctx context.Context, s *Server) {
		if err := s.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.LogAttrs(ctx, slog.LevelError, "error while running server", slog.Any("err", err))
			os.Exit(1)
			return
		}
	}(ctx, s)

	<-ctx.Done()
	slog.LogAttrs(ctx, slog.LevelInfo, "shutting down...")
}

// getListener returns the net.Listener to listen on.
//
// This function will automatically detect if we are running under systemd with a socket,
// so we can "bind" to privileged ports without needing any privileges ourselves.
//
// If we are not running with a systemd socket activation, we will bind to the address set by
// `CLI.Listen`.
func getListener(ctx context.Context) (net.Listener, error) {
	listeners, err := systemd.Listeners()
	if err != nil {
		return nil, fmt.Errorf("failed to get systemd listeners: %w", err)
	}
	if len(listeners) == 1 {
		return listeners[0], nil
	}
	return (&net.ListenConfig{}).Listen(ctx, "tcp", CLI.Listen)
}

// Server is responsible for listening for incoming connections, reading data, and forwarding it
// to a haste-server.
type Server struct {
	listener net.Listener
	haste    *haste.Client
}

// NewServer returns a new server using the provided listener and haste-server client.
func NewServer(l net.Listener, h *haste.Client) *Server {
	return &Server{
		listener: l,
		haste:    h,
	}
}

// Run runs the server, listening for incoming connections on the server's listener.
func (s *Server) Run(ctx context.Context) error {
	slog.LogAttrs(ctx, slog.LevelInfo, "listening for incoming connections...")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				// Ignore `use of closed network connection` errors, these are triggered when the
				// server is shutting down.
				if strings.HasSuffix(err.Error(), "use of closed network connection") {
					break
				}
				slog.LogAttrs(ctx, slog.LevelWarn, "error while accepting connection", slog.Any("err", err))
				break
			}

			// Handle the connection in the background.
			go func(ctx context.Context, conn net.Conn) {
				if err := s.handle(ctx, conn); err != nil {
					slog.LogAttrs(ctx, slog.LevelWarn, "error while handling connection", slog.Any("err", err))
				}
			}(ctx, conn)
		}
	}
}

// handle handles an incoming connection from the listener.
func (s *Server) handle(ctx context.Context, conn net.Conn) error {
	remoteAddr := conn.RemoteAddr().String()
	slog.LogAttrs(ctx, slog.LevelInfo, "new connection", slog.Any("remote_addr", remoteAddr))
	defer slog.LogAttrs(ctx, slog.LevelInfo, "connection closed", slog.Any("remote_addr", remoteAddr))
	defer conn.Close()

	// buf is all the data read from the connection.
	var buf bytes.Buffer
	// tmp is used to read smaller chunks of data from the connection.
	tmp := make([]byte, 1024)
	for {
		// Reset the read deadline on each iteration, this functions as a timeout for each read.
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}

		n, err := conn.Read(tmp)
		if err != nil {
			// Normally you would wait for an io.EOF here, but netcat doesn't send an EOF when it's
			// finished, so we just have to assume that it finished sending data after a timeout
			// is reached.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if buf.Len() < 1 {
					slog.LogAttrs(ctx, slog.LevelInfo, "no data received from client before connection timed out")
					return nil
				}

				// Got data from client, break.
				break
			}
		}

		buf.Write(tmp[:n])
		if buf.Len() > CLI.Limit {
			if err := conn.SetWriteDeadline(time.Now().Add(1 * time.Second)); err != nil {
				return fmt.Errorf("failed to set write deadline: %w", err)
			}
			// TODO: it would be nice if we could pretty print the limit rather than always sending
			// it as the number of bytes.
			_, err = conn.Write([]byte("Pastes may not exceed " + strconv.Itoa(CLI.Limit) + " bytes of data"))
			return err
		}
	}

	// Send the data to the haste-server.
	r, err := s.haste.Paste(ctx, &buf)
	if err != nil {
		return fmt.Errorf("failed to forward data to hastebin: %w", err)
	}

	// Stupidly, but efficiently do byte slice copies to combine the URL and Key into a single
	// URL to write back to the client.
	url := []byte(s.haste.URL)
	key := []byte(r.Key)
	res := make([]byte, len(url)+len(key)+2)
	n := copy(res, url)
	res[n] = '/'
	n++
	n += copy(res[n:], key)
	res[n] = '\n'

	if err := conn.SetWriteDeadline(time.Now().Add(1 * time.Second)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}
	_, err = conn.Write(res)
	return err
}
