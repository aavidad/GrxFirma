// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package identityevidence

import (
	"os"
	"path/filepath"
	"testing"
)

func assertPermisosRegistro(t *testing.T, path string) {
	t.Helper()
	for ruta, modo := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		info, err := os.Stat(ruta)
		if err != nil || info.Mode().Perm() != modo {
			t.Fatalf("permisos inseguros en %s: info=%v error=%v", ruta, info, err)
		}
	}
}

func TestRegistroRechazaPermisosPOSIXAmpliados(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "fichero", true: "directorio"}[directory], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "privado", "registro.log")
			config := Configuracion{Ruta: path, Clave: make([]byte, 32), VersionClave: "v1"}
			if _, err := Nuevo(config); err != nil {
				t.Fatal(err)
			}
			target, mode := path, os.FileMode(0o644)
			if directory {
				target, mode = filepath.Dir(path), 0o755
			}
			if err := os.Chmod(target, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := Nuevo(config); err == nil {
				t.Fatal("aceptó permisos ampliados")
			}
		})
	}
}
