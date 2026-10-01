// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"os"

	"golang.org/x/sys/unix"
)

func protectRegularFile(path string, perm os.FileMode) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open devuelve un descriptor no negativo.
	if file == nil {
		_ = unix.Close(fd)
		return os.ErrInvalid
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrInvalid
	}
	return file.Chmod(perm.Perm())
}
