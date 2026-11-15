//go:build !windows

// SPDX-License-Identifier: EUPL-1.2
package settingsfile

import "os"

func protectSettingsDirectory(dir *os.File) error {
	return dir.Chmod(0700)
}
