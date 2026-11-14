// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSensitiveFileBorraElFichero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secreto.json")
	if err := os.WriteFile(path, []byte("clave privada en claro"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSensitiveFile(path); err != nil {
		t.Fatalf("RemoveSensitiveFile() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el fichero sigue existiendo: %v", err)
	}
}

func TestRemoveSensitiveFileSobrescribeAntesDeBorrar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secreto.json")
	if err := os.WriteFile(path, []byte("clave privada en claro"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Un enlace duro conserva el contenido tras borrar el nombre original:
	// permite comprobar que se sobrescribió antes de borrar.
	testigo := filepath.Join(dir, "testigo")
	if err := os.Link(path, testigo); err != nil {
		t.Skipf("enlaces duros no disponibles: %v", err)
	}
	if err := RemoveSensitiveFile(path); err != nil {
		t.Fatalf("RemoveSensitiveFile() error = %v", err)
	}
	data, err := os.ReadFile(testigo)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range data {
		if b != 0 {
			t.Fatalf("el contenido no se sobrescribió con ceros: %q", data)
		}
	}
}

func TestRemoveSensitiveFileNoSigueEnlaces(t *testing.T) {
	dir := t.TempDir()
	victima := filepath.Join(dir, "victima")
	if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
		t.Fatal(err)
	}
	enlace := filepath.Join(dir, "enlace")
	if err := os.Symlink(victima, enlace); err != nil {
		t.Skipf("enlaces simbólicos no disponibles: %v", err)
	}
	if err := RemoveSensitiveFile(enlace); err == nil {
		t.Fatal("RemoveSensitiveFile() aceptó un enlace simbólico")
	}
	data, err := os.ReadFile(victima)
	if err != nil || string(data) != "intacto" {
		t.Fatalf("la víctima cambió: %q, %v", data, err)
	}
}
