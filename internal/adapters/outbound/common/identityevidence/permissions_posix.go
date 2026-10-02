// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package identityevidence

import (
	"os"

	"golang.org/x/sys/unix"
)

func prepararDirectorioPrivado(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return comprobarPermisosPrivados(path, true)
}

func abrirFicheroRegistro(path string, crear bool) (*os.File, error) {
	flags := unix.O_WRONLY | unix.O_APPEND | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if crear {
		flags |= unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Open(path, flags, 0o600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path) // #nosec G115 -- unix.Open returned a non-negative descriptor.
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		_ = f.Close()
		return nil, ErrConfiguracionInvalida
	}
	return f, nil
}

func comprobarPermisosPrivados(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if directory {
		if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return ErrConfiguracionInvalida
		}
	} else if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return ErrConfiguracionInvalida
	}
	return nil
}
