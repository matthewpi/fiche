// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2024 Matthew Penner

package haste

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client represents a Hastebin API client.
type Client struct {
	// URL of the Hastebin instance.
	URL string

	http *http.Client
}

// NewClient returns a new Hastebin client.
func NewClient(url string) (*Client, error) {
	return &Client{
		URL:  strings.TrimSuffix(url, "/"),
		http: &http.Client{},
	}, nil
}

// PasteResponse is the response from a Paste request.
type PasteResponse struct {
	Key string `json:"key"`
}

// Paste sends a paste to the haste-server.
func (c *Client) Paste(ctx context.Context, r io.Reader) (*PasteResponse, error) {
	// Send a request to the hastebin instance to create a new paste.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/documents", r)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "github.com/matthewpi/fiche")

	// Run the request
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute http request: %w", err)
	}
	defer res.Body.Close()

	// Handle non 200 and 201 status codes.
	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusCreated {
		return nil, newStatusError(res, http.StatusOK)
	}

	// Decode the response.
	var paste PasteResponse
	if err := json.NewDecoder(res.Body).Decode(&paste); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return &paste, nil
}
