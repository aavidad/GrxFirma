//go:build linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadOwnedSystemFileLimit_EnlaceProtegido(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(root, "certificado.pem")
	link := filepath.Join(root, "enlace.pem")
	if err := os.WriteFile(actual, []byte("certificado sintético"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileLimit(link, 1024); !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("ReadFileLimit(enlace) = %v; se esperaba ELOOP", err)
	}
	data, err := readOwnedSystemFileLimit(link, 1024, uint32(os.Geteuid()), root)
	if err != nil || string(data) != "certificado sintético" {
		t.Fatalf("lectura protegida = %q, %v", data, err)
	}
	if err := os.Chmod(actual, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := readOwnedSystemFileLimit(link, 1024, uint32(os.Geteuid()), root); err == nil {
		t.Fatal("se aceptó un destino escribible por otros")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "externo.pem"), []byte("no confiable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "externo.pem"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := readOwnedSystemFileLimit(link, 1024, uint32(os.Geteuid()), root); err == nil {
		t.Fatal("se aceptó un enlace fuera del árbol administrado")
	}
}
