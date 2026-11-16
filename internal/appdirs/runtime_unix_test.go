// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package appdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeUsuarioAdmiteXDGPrivado(t *testing.T) {
	prepararPerfil(t)
	xdg := t.TempDir()
	if err := os.Chmod(xdg, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	if dir, err := RuntimeUsuario(); err != nil || dir != xdg {
		t.Fatalf("XDG privado no admitido: %q, %v", dir, err)
	}
}

func TestRuntimeUsuarioRechazaXDGCompartidoOEnlace(t *testing.T) {
	home := prepararPerfil(t)
	compartido := t.TempDir()
	if err := os.Chmod(compartido, 0o777); err != nil {
		t.Fatal(err)
	}
	privado := t.TempDir()
	enlace := filepath.Join(t.TempDir(), "enlace")
	if err := os.Symlink(privado, enlace); err != nil {
		t.Fatal(err)
	}
	for _, xdg := range []string{compartido, enlace, filepath.Join(privado, "no-existe")} {
		t.Setenv("XDG_RUNTIME_DIR", xdg)
		dir, err := RuntimeUsuario()
		if err != nil || dir != filepath.Join(home, ".local", "run") {
			t.Fatalf("XDG_RUNTIME_DIR=%q admitido: %q, %v", xdg, dir, err)
		}
	}
}

func TestRuntimeUsuarioRechazaXDGDeOtroUsuario(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root es propietario de todo")
	}
	home := prepararPerfil(t)
	// / pertenece a root: es una carpeta ajena aunque exista.
	t.Setenv("XDG_RUNTIME_DIR", "/")
	dir, err := RuntimeUsuario()
	if err != nil || dir != filepath.Join(home, ".local", "run") {
		t.Fatalf("XDG ajeno admitido: %q, %v", dir, err)
	}
}
