// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"fmt"
	"os"
)

func removeRegularFileNoFollow(path string) error {
	file, err := OpenRead(path)
	if err != nil {
		return err
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("securefile: stat opened file before removal: %w", err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 ||
		!pathInfo.Mode().IsRegular() ||
		!os.SameFile(openedInfo, pathInfo) {
		return fmt.Errorf("securefile: path changed before removal: %s", path)
	}
	return os.Remove(path)
}
