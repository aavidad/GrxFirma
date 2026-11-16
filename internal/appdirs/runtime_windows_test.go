// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package appdirs

import "testing"

func TestRuntimeUsuarioIgnoraXDGEnWindows(t *testing.T) {
	home := prepararPerfil(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if dir, err := RuntimeUsuario(); err != nil || dir != reservaEsperada(home) {
		t.Fatalf("XDG_RUNTIME_DIR usado en Windows: %q, %v", dir, err)
	}
	if dir, ok := XDGRuntimePrivado(); ok {
		t.Fatalf("XDG_RUNTIME_DIR admitido en Windows: %q", dir)
	}
}
