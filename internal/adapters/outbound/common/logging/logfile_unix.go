// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package logging

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func abrirLogAppendSeguro(ruta string) (*os.File, error) {
	// ruta is absolute, its parent chain was validated with Lstat, and
	// O_NOFOLLOW prevents a final-component symlink swap before open.
	fd, err := unix.Open(ruta, unix.O_CREAT|unix.O_WRONLY|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el fichero de log sin seguir enlaces: %w", err)
	}
	// unix.Open returned a non-negative descriptor; the conversion preserves
	// that OS handle exactly.
	fichero := os.NewFile(uintptr(fd), ruta) // #nosec G115 -- unix.Open only returns non-negative descriptors on success.
	if fichero == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("no se pudo representar el fichero de log abierto")
	}
	if err := validarLogAbierto(fichero, ruta); err != nil {
		_ = fichero.Close()
		return nil, err
	}
	return fichero, nil
}
