// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2024 Matthew Penner

package haste

import (
	"fmt"
	"io"
	"net/http"
)

// StatusError indicates an HTTP request failure with a status code from a remote
// HTTP server.
type StatusError struct {
	// Data from the response.
	Data []byte

	// StatusCode from the response.
	StatusCode int

	// Expected StatusCode the response should've had.
	Expected int
}

var _ error = StatusError{}

// newStatusError returns a new status error using information from the request.
func newStatusError(res *http.Response, expected int) StatusError {
	e := StatusError{
		StatusCode: res.StatusCode,
		Expected:   expected,
	}
	if res.Body != nil {
		defer func() {
			_ = res.Body.Close()
		}()
		body := io.LimitReader(res.Body, 4*1024)
		if b, err := io.ReadAll(body); err == nil {
			e.Data = b
		}
	}
	return e
}

// Error satisfies the error interface.
func (e StatusError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("expected %d status code, but got %d (%s)", e.Expected, e.StatusCode, string(e.Data))
	}
	return fmt.Sprintf("expected %d status code, but got %d", e.Expected, e.StatusCode)
}
