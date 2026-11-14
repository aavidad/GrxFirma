// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openWriteExistingNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open-write-nofollow", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open devuelve un descriptor no negativo.
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("securefile: could not represent opened file")
	}
	return file, nil
}
