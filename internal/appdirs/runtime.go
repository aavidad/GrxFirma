// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package appdirs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrRuntimeNoDisponible indica que no hay una carpeta privada del usuario
// para sockets y ficheros de sesión.
var ErrRuntimeNoDisponible = errors.New("appdirs: carpeta privada de ejecución no disponible")

// RuntimeUsuario devuelve la carpeta privada del usuario para sockets y
// ficheros que solo valen mientras dura la sesión.
//
// Usa XDG_RUNTIME_DIR solo si es una carpeta real (no un enlace) y, en Unix,
// del usuario actual y sin permisos para grupo ni otros. Si no lo es, o falta,
// usa una
// carpeta dentro de la carpeta personal: ~/.local/run en Unix y
// %LOCALAPPDATA%\GrxFirma\run en Windows. Esa carpeta de reserva no se crea
// aquí; quien escribe la crea con EnsurePrivateDir de securefile.
//
// Nunca devuelve la carpeta temporal compartida: en /tmp otro usuario podría
// crear la ruta antes y quedarse con el socket o plantar preferencias.
func RuntimeUsuario() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); dir != "" && xdgRuntimeAdmitido(dir) {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", ErrRuntimeNoDisponible
	}
	return runtimeReserva(home), nil
}

func xdgRuntimeAdmitido(dir string) bool {
	if !filepath.IsAbs(dir) || strings.IndexByte(dir, 0) >= 0 {
		return false
	}
	return carpetaPrivadaDelUsuario(filepath.Clean(dir))
}
