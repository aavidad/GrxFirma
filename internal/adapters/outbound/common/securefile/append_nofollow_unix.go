// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openAppendNoFollow(path string, perm os.FileMode) (*os.File, error) {
	fd, err := unix.Open(
		path,
		unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_CLOEXEC|
			unix.O_NOFOLLOW|unix.O_NONBLOCK,
		uint32(perm.Perm()),
	)
	if err != nil {
		return nil, &os.PathError{Op: "open-append-nofollow", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open devuelve un descriptor no negativo.
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("securefile: could not represent opened append file")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("securefile: stat append file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("securefile: append path is not a regular file: %s", path)
	}
	if err := file.Chmod(perm.Perm()); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("securefile: restrict append file: %w", err)
	}
	return file, nil
}
