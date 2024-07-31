// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright (c) 2015 CoreOS, Inc.

package systemd

import "os"

func Files() []*os.File {
	// On Windows this operation is a no-op.
	return nil
}
