// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package appdirs

import (
	"os"
	"path/filepath"
	"syscall"
)

func runtimeReserva(home string) string {
	return filepath.Join(home, ".local", "run")
}

// carpetaPrivadaDelUsuario comprueba sin seguir enlaces que dir es una
// carpeta del usuario efectivo y que grupo y otros no tienen permisos.
func carpetaPrivadaDelUsuario(dir string) bool {
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return uint64(st.Uid) == uint64(os.Geteuid()) && info.Mode().Perm()&0o077 == 0 // #nosec G115 -- geteuid nunca es negativo y uint32 cabe en uint64.
}
