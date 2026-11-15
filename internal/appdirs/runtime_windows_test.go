// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package appdirs

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRuntimeUsuarioWindowsAdmiteXDGCarpetaReal(t *testing.T) {
	prepararPerfil(t)
	xdg := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	if dir, err := RuntimeUsuario(); err != nil || dir != xdg {
		t.Fatalf("XDG_RUNTIME_DIR real no admitido: %q, %v", dir, err)
	}
}

func TestRuntimeUsuarioWindowsRechazaXDGUnion(t *testing.T) {
	home := prepararPerfil(t)
	destino := t.TempDir()
	union := filepath.Join(t.TempDir(), "union")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", union, destino).CombinedOutput(); err != nil {
		t.Skipf("no se pudo crear la unión: %v %s", err, out)
	}
	t.Setenv("XDG_RUNTIME_DIR", union)
	if dir, err := RuntimeUsuario(); err != nil || dir != reservaEsperada(home) {
		t.Fatalf("unión admitida como XDG_RUNTIME_DIR: %q, %v", dir, err)
	}
}
