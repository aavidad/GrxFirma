// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package appdirs resuelve los directorios propios de GrxFirma.
package appdirs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config devuelve la raíz de configuración del usuario.
func Config(home string) string {
	if runtime.GOOS == "windows" {
		return windowsRoot(home, "APPDATA", "Roaming")
	}
	return filepath.Join(home, ".config", "grxfirma")
}

// ErrConfigNoDisponible indica que el entorno no da una carpeta de
// configuración absoluta y fiable.
var ErrConfigNoDisponible = errors.New("appdirs: carpeta de configuración no disponible")

// ConfigComprobada es Config para quien debe fallar cerrado. Rechaza una
// carpeta personal vacía o relativa y, en Windows, también %APPDATA% vacío o
// relativo, que allí decide la configuración igual que HOME en Linux. Config
// tomaría entonces %USERPROFILE%\AppData\Roaming por su cuenta.
func ConfigComprobada(home string) (string, error) {
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", ErrConfigNoDisponible
	}
	if runtime.GOOS == "windows" && !filepath.IsAbs(os.Getenv("APPDATA")) {
		return "", ErrConfigNoDisponible
	}
	return Config(home), nil
}

// ConfigUsuario es ConfigComprobada con la carpeta personal del sistema.
func ConfigUsuario() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ErrConfigNoDisponible
	}
	return ConfigComprobada(home)
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
