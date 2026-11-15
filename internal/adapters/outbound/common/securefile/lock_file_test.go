// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile_test

import (
	"bytes"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenLockFileReadsWritesAndPreservesContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.lock")
	f, err := securefile.OpenLockFile(path, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("cerrojo"), 0); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 7)
	if _, err := f.ReadAt(data, 0); err != nil || !bytes.Equal(data, []byte("cerrojo")) {
		t.Fatalf("lectura=%q err=%v", data, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = securefile.OpenLockFile(path, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() != 7 {
		t.Fatalf("se truncó: %v %v", info, err)
	}
}
func TestOpenLockFileRejectsDirectoryAndSymlink(t *testing.T) {
	dir := t.TempDir()
	if f, err := securefile.OpenLockFile(dir, 0600); err == nil {
		f.Close()
		t.Fatal("directorio aceptado")
	}
	target := filepath.Join(dir, "original")
	if err := os.WriteFile(target, []byte("no tocar"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "enlace")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink no disponible: %v", err)
	}
	if f, err := securefile.OpenLockFile(link, 0600); err == nil {
		f.Close()
		t.Fatal("enlace aceptado")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "no tocar" {
		t.Fatalf("destino alterado: %q %v", data, err)
	}
}
