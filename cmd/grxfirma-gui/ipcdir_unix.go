// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// comprobarDirectorioPrivado exige un directorio real (no enlace) del usuario
// actual sin permisos para el grupo ni para otros. Con corregirPermisos, un
// directorio propio con permisos más abiertos se deja en 0700.
func comprobarDirectorioPrivado(dir string, corregirPermisos bool) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s no es un directorio", dir)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("no se pudo leer el propietario del directorio IPC")
	}
	if uint64(st.Uid) != uint64(os.Geteuid()) { // #nosec G115 -- geteuid nunca es negativo; uint32 cabe en uint64
		return fmt.Errorf("el directorio %s no pertenece al usuario actual", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		if !corregirPermisos {
			return fmt.Errorf("el directorio %s no es privado", dir)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
