// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package appdirs resuelve los directorios propios de GrxFirma.
package appdirs

import (
	"os"
	"path/filepath"
	"runtime"
)

// Config devuelve la raíz de configuración del usuario.
func Config(home string) string {
	if runtime.GOOS == "windows" {
		return windowsRoot(home, "APPDATA", "Roaming")
	}
	return filepath.Join(home, ".config", "grxfirma")
}

// Data devuelve la raíz de datos del usuario.
func Data(home string) string {
	if runtime.GOOS == "windows" {
		return windowsRoot(home, "LOCALAPPDATA", "Local")
	}
	return filepath.Join(home, ".local", "share", "grxfirma")
}

// State devuelve la raíz de diagnósticos y registros del usuario.
func State(home string) string {
	if runtime.GOOS == "windows" {
		return windowsRoot(home, "LOCALAPPDATA", "Local")
	}
	return filepath.Join(home, ".local", "state", "grxfirma")
}

// Cache devuelve la raíz de ficheros temporales persistentes del usuario.
func Cache(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(windowsRoot(home, "LOCALAPPDATA", "Local"), "Cache")
	}
	return filepath.Join(home, ".cache", "grxfirma")
}

func windowsRoot(home, variable, subdir string) string {
	base := os.Getenv(variable)
	if !filepath.IsAbs(base) {
		base = filepath.Join(home, "AppData", subdir)
	}
	return filepath.Join(base, "GrxFirma")
}
