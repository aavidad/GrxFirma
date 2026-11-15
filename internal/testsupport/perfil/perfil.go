// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package perfil aísla el perfil del usuario en las pruebas.
//
// Cambiar solo HOME no basta: en Windows la carpeta personal sale de
// USERPROFILE y la configuración de GrxFirma de %APPDATA%, y una prueba que
// solo cambia HOME acaba leyendo y escribiendo los ajustes reales de quien
// ejecuta go test. Aislar redirige a la vez todas las variables que deciden
// el perfil en Linux, macOS y Windows.
package perfil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"grxfirma/internal/appdirs"
)

// Aislar hace que home sea el perfil del usuario durante la prueba en todos
// los sistemas y lo devuelve en forma absoluta. Las variables se restauran al
// terminar la prueba (t.Setenv), por lo que no admite pruebas en paralelo.
func Aislar(t testing.TB, home string) string {
	t.Helper()
	abs, err := filepath.Abs(home)
	if err != nil {
		t.Fatalf("perfil: ruta del perfil: %v", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		t.Fatalf("perfil: crear el perfil: %v", err)
	}
	volume := filepath.VolumeName(abs)
	for name, value := range map[string]string{
		"HOME":            abs,
		"USERPROFILE":     abs,
		"HOMEDRIVE":       volume,
		"HOMEPATH":        strings.TrimPrefix(abs, volume),
		"APPDATA":         filepath.Join(abs, "AppData", "Roaming"),
		"LOCALAPPDATA":    filepath.Join(abs, "AppData", "Local"),
		"XDG_CONFIG_HOME": filepath.Join(abs, ".config"),
		"XDG_DATA_HOME":   filepath.Join(abs, ".local", "share"),
		"XDG_STATE_HOME":  filepath.Join(abs, ".local", "state"),
		"XDG_CACHE_HOME":  filepath.Join(abs, ".cache"),
	} {
		t.Setenv(name, value)
	}
	got, err := os.UserHomeDir()
	if err != nil || !mismaRuta(got, abs) {
		t.Fatalf("perfil: os.UserHomeDir() = %q, %v; se esperaba %q", got, err, abs)
	}
	return abs
}

// Nuevo crea un perfil vacío en una carpeta temporal de la prueba y lo aísla.
func Nuevo(t testing.TB) string {
	t.Helper()
	return Aislar(t, t.TempDir())
}

// DirConfig devuelve la carpeta de configuración de GrxFirma dentro del
// perfil aislado, con la misma regla que la aplicación.
func DirConfig(home string) string {
	return appdirs.Config(home)
}

func mismaRuta(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
