// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import "os"

func protectOpenedDirectory(dir *os.File, perm os.FileMode) error {
	return dir.Chmod(perm.Perm())
}
