// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package appdirs

import (
	"os"
	"path/filepath"
)

func runtimeReserva(home string) string {
	return filepath.Join(windowsRoot(home, "LOCALAPPDATA", "Local"), "run")
}

// En Windows el sistema no fija XDG_RUNTIME_DIR; si el usuario lo define se
// respeta como antes, siempre que sea una carpeta real y no un enlace ni una
// unión. Los permisos no se pueden comprobar con bits POSIX: quien escribe
// dentro aplica la DACL privada con securefile.EnsurePrivateDir.
func carpetaPrivadaDelUsuario(dir string) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	return info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}
