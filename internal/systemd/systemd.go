// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright (c) 2015 CoreOS, Inc.

package systemd

import "net"

// Listeners returns a slice containing a net.Listener for each matching socket type
// passed to this process.
//
// The order of the file descriptors is preserved in the returned slice.
// Nil values are used to fill any gaps. For example if systemd were to return file descriptors
// corresponding with "udp, tcp, tcp", then the slice would contain {nil, net.Listener, net.Listener}
func Listeners() ([]net.Listener, error) {
	files := Files()
	listeners := make([]net.Listener, len(files))

	for i, f := range files {
		if pc, err := net.FileListener(f); err == nil {
			listeners[i] = pc
			_ = f.Close()
		}
	}
	return listeners, nil
}
