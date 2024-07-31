// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright (c) 2015 CoreOS, Inc.

package systemd

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	// listenFdsStart corresponds to `SD_LISTEN_FDS_START`.
	listenFdsStart = 3
)

func Files() []*os.File {
	pid, err := strconv.Atoi(os.Getenv("LISTEN_PID"))
	if err != nil || pid != os.Getpid() {
		return nil
	}

	nfds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || nfds == 0 {
		return nil
	}

	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")

	files := make([]*os.File, 0, nfds)
	for fd := listenFdsStart; fd < listenFdsStart+nfds; fd++ {
		syscall.CloseOnExec(fd)
		name := "LISTEN_FD_" + strconv.Itoa(fd)
		offset := fd - listenFdsStart
		if offset < len(names) && len(names[offset]) > 0 {
			name = names[offset]
		}
		files = append(files, os.NewFile(uintptr(fd), name))
	}

	return files
}
