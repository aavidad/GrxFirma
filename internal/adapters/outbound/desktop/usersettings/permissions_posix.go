// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package usersettings

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func protegerDirectorioConfiguracion(path string) error {
	return protegerRutaConfiguracion(path, 0o700, true)
}

func protegerFicheroConfiguracion(path string) error {
	return protegerRutaConfiguracion(path, 0o600, false)
}

func protegerRutaConfiguracion(path string, mode os.FileMode, directorio bool) (err error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open returns a non-negative descriptor on success.
	if f == nil {
		_ = unix.Close(fd)
		return errors.New("no se pudo representar la ruta de configuracion abierta")
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() != directorio {
		if directorio {
			return errors.New("la ruta de configuracion no es un directorio")
		}
		return errors.New("la ruta de configuracion no es un fichero")
	}
	return f.Chmod(mode)
}
