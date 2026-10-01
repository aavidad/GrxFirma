// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsolatedModuleRejectsUnconfinedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic-module")
	if err := os.WriteFile(path, []byte("not a driver"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"", "/", "relative", path, link, dir, "/proc/self/exe", "/dev/null", path + "/../synthetic-module", path + "\x00"} {
		if _, err := ValidateIsolatedModulePath(candidate); !errors.Is(err, ErrSandboxPolicy) {
			t.Fatalf("unconfined or invalid path accepted: %v", err)
		}
	}
}
