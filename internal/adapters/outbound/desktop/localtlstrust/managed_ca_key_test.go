// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManagedCAKey_ProteccionYLectura(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "root.key.pem")
	key := []byte("clave sintética de prueba")
	if err := SaveManagedCAKey(path, key); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && bytes.Contains(stored, key) {
		t.Fatal("la clave CA quedó legible sin DPAPI")
	}
	loaded, err := LoadManagedCAKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, key) {
		t.Fatal("la clave CA recuperada no coincide")
	}
	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(dir)
		if err != nil || dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("directorio TLS sin 0700: %v, %v", dirInfo, err)
		}
		fileInfo, err := os.Stat(path)
		if err != nil || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("clave CA sin 0600: %v, %v", fileInfo, err)
		}
	}
}
