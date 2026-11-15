//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"grxfirma/internal/testsupport/perfil"
)

func TestWindowsUnionNoPermiteLeerDatosPrivados(t *testing.T) {
	home := perfil.Nuevo(t)
	roaming := filepath.Join(home, "AppData", "Roaming")
	local := filepath.Join(home, "AppData", "Local")
	for _, pair := range [][2]string{{"Application Data", roaming}, {"Local Settings", local}} {
		private := filepath.Join(pair[1], "GrxFirma")
		if err := os.MkdirAll(private, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(private, "settings.json"), []byte("privado"), 0600); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(home, pair[0])
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", alias, pair[1]).CombinedOutput(); err != nil {
			t.Fatalf("crear unión: %v %s", err, out)
		}
		path := filepath.Join(alias, "GrxFirma", "settings.json")
		if _, err := Check(path); !errors.Is(err, ErrProtected) {
			t.Fatalf("unión admitida: %s %v", path, err)
		}
		if _, err := ReadFileLimit(path, 1024); !errors.Is(err, ErrProtected) {
			t.Fatalf("datos expuestos por unión: %v", err)
		}
	}
}

// Una unión de Windows se crea sin privilegios, a diferencia de un enlace
// simbólico: guardar a través de ella tampoco puede salir de la carpeta
// elegida, aunque el destino no esté protegido.
func TestWindowsUnionNoSeSigueAlGuardar(t *testing.T) {
	home := perfil.Nuevo(t)
	outside := t.TempDir()
	alias := filepath.Join(home, "Descargas")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", alias, outside).CombinedOutput(); err != nil {
		t.Fatalf("crear unión: %v %s", err, out)
	}
	for _, force := range []bool{false, true} {
		if got, err := WriteFile(filepath.Join(alias, "escape.txt"), []byte("contenido"), force); err == nil {
			t.Fatalf("force=%v: se guardó a través de la unión en %q", force, got)
		}
		if _, err := os.Lstat(filepath.Join(outside, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("force=%v: se escribió en el destino de la unión: %v", force, err)
		}
	}
}
