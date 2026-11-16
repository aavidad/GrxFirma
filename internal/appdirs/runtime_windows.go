// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package appdirs

import "path/filepath"

func runtimeReserva(home string) string {
	return filepath.Join(windowsRoot(home, "LOCALAPPDATA", "Local"), "run")
}

// En Windows el sistema no fija XDG_RUNTIME_DIR y sus permisos no se pueden
// comprobar con bits POSIX: se ignora y se usa siempre la carpeta local del
// perfil, que securefile.EnsurePrivateDir deja con una DACL privada.
func carpetaPrivadaDelUsuario(string) bool {
	return false
}
