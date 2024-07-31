// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2024 Matthew Penner

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/matthewpi/fiche/internal/haste"
	"github.com/matthewpi/fiche/internal/systemd"
)

var CLI struct {
	Listen   string `help:"Listen address" default:":99"`
	Hastebin string `help:"haste-server URL" placeholder:"https://ptero.co"`
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

type Server struct {
	listener net.Listener
	haste    *haste.Client
}

func NewServer(l net.Listener, h *haste.Client) *Server {
	return &Server{
		listener: l,
		haste:    h,
	}
}

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

			go func(ctx context.Context, conn net.Conn) {
				if err := s.handle(ctx, conn); err != nil {
					slog.LogAttrs(ctx, slog.LevelWarn, "error while handling connection", slog.Any("err", err))
				}
			}(ctx, conn)
		}
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) error {
	slog.LogAttrs(ctx, slog.LevelInfo, "new connection")
	defer slog.LogAttrs(ctx, slog.LevelInfo, "connection closed")
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	r, err := s.haste.Paste(ctx, io.LimitReader(conn, 1024))
	if err != nil {
		return fmt.Errorf("failed to forward data to hastebin: %w", err)
	}

	url := []byte(s.haste.URL)
	key := []byte(r.Key)

	// Stupidly, but efficiently do byte slice copies to combine the URL and Key into a single
	// URL to write back to the client.
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
