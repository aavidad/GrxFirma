// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package machinepolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPoliticaUnixExigePropietarioYRutaSegura(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Getuid())
	dir := filepath.Join(root, "grxfirma")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "policy.json")
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"aprobacion_automatica_nativehost":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	read := func(path string, owner uint32) error {
		_, err := readPolicyFileFrom(path, root, owner, 1024)
		return err
	}
	if err := read(path, owner); err != nil {
		t.Fatalf("política segura rechazada: %v", err)
	}
	if err := read(path, owner+1); err == nil {
		t.Fatal("aceptó política sin el propietario exigido")
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := read(path, owner); err == nil {
		t.Fatal("aceptó un ancestro escribible por otros")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o622); err != nil {
		t.Fatal(err)
	}
	if err := read(path, owner); err == nil {
		t.Fatal("aceptó un fichero escribible por otros")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "enlace.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := read(link, owner); err == nil {
		t.Fatal("aceptó un enlace simbólico en el fichero")
	}
	linkDir := filepath.Join(root, "enlace")
	if err := os.Symlink(dir, linkDir); err != nil {
		t.Fatal(err)
	}
	if err := read(filepath.Join(linkDir, "policy.json"), owner); err == nil {
		t.Fatal("aceptó un enlace simbólico en un ancestro")
	}
}
