// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/certselectionstore"
)

// prepararSesionAutoseleccionPrueba aísla la carpeta de ejecución dentro de
// tmp (XDG_RUNTIME_DIR en Unix, %LOCALAPPDATA% en Windows, que ignora XDG),
// deja privadas sus carpetas y devuelve la ruta del fichero de sesión que usa
// el motor.
func prepararSesionAutoseleccionPrueba(t *testing.T, tmp string) string {
	t.Helper()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "AppData", "Local"))
	}
	ruta, err := certselectionstore.SessionPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := certselectionstore.WriteSessionFile(ruta, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	return ruta
}
