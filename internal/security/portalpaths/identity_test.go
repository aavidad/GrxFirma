// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtectedAncestorComparaIdentidadesYFallaCerrado(t *testing.T) {
	real := t.TempDir()
	info, err := os.Stat(real)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "ALIAS")
	stat := func(path string) (os.FileInfo, error) {
		if path == alias {
			return os.Stat(real)
		}
		return nil, os.ErrNotExist
	}
	blocked, err := protectedAncestor(filepath.Join(alias, "nuevo", "settings.json"), []os.FileInfo{info}, stat)
	if err != nil || !blocked {
		t.Fatalf("alias por identidad no protegido: %v %v", blocked, err)
	}
	if blocked, err := protectedAncestor(alias, []os.FileInfo{info}, func(string) (os.FileInfo, error) { return nil, os.ErrPermission }); err == nil || blocked {
		t.Fatalf("error de identidad no se propagó: %v %v", blocked, err)
	}
}
