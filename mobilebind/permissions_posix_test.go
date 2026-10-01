// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package mobilebind

import (
	"os"
	"testing"
)

func assertPrivateDirectoryMode(t *testing.T, info os.FileInfo) {
	t.Helper()
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("permisos del directorio privado = %o, want 700", info.Mode().Perm())
	}
}
