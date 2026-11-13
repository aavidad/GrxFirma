// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package ipc

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// zonasSistemaProhibidas lista las carpetas de Windows que el IPC no lee ni
// escribe. Se obtienen de la API del sistema, no de variables de entorno. Es
// una lista de robustez, no un control de acceso.
func zonasSistemaProhibidas() []string {
	var zonas []string
	if dir, err := windows.GetWindowsDirectory(); err == nil && filepath.IsAbs(dir) {
		zonas = append(zonas, filepath.Clean(dir))
	}
	if dir, err := windows.GetSystemDirectory(); err == nil && filepath.IsAbs(dir) {
		zonas = append(zonas, filepath.Clean(dir))
	}
	return zonas
}
