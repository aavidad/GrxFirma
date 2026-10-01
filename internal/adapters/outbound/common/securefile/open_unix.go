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

func openReadNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open-nofollow", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open returns a non-negative descriptor on success.
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("securefile: could not represent opened file")
	}
	return file, nil
}

func openDirNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open-dir-nofollow", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open returns a non-negative descriptor on success.
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("securefile: could not represent opened directory")
	}
	return file, nil
}
