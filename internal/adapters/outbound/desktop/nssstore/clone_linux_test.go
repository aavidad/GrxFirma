// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClonarBaseNSSTemporal_IgnoraLockFirefoxYFicherosAjenos(t *testing.T) {
	perfil := t.TempDir()
	archivosNSS := map[string]string{
		"cert9.db":     "certificados",
		"cert9.db-wal": "wal certificados",
		"cert9.db-shm": "shm certificados",
		"key4.db":      "claves",
		"key4.db-wal":  "wal claves",
		"key4.db-shm":  "shm claves",
		"pkcs11.txt":   "modulos",
	}
	for nombre, contenido := range archivosNSS {
		if err := os.WriteFile(filepath.Join(perfil, nombre), []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, nombre := range []string{"lock", ".parentlock"} {
		if err := os.Symlink("127.0.1.1:+1234", filepath.Join(perfil, nombre)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(perfil, "places.sqlite"), []byte("datos ajenos"), 0o600); err != nil {
		t.Fatal(err)
	}

	clon, err := clonarBaseNSSTemporal(perfil)
	if err != nil {
		t.Fatalf("clonar perfil con lock Firefox: %v", err)
	}
	defer os.RemoveAll(clon)
	for nombre, contenido := range archivosNSS {
		leido, err := os.ReadFile(filepath.Join(clon, nombre))
		if err != nil || string(leido) != contenido {
			t.Fatalf("fichero NSS %q: contenido %q, error %v", nombre, leido, err)
		}
	}
	for _, nombre := range []string{"lock", ".parentlock", "places.sqlite"} {
		if _, err := os.Lstat(filepath.Join(clon, nombre)); !os.IsNotExist(err) {
			t.Fatalf("el clon contiene %q: %v", nombre, err)
		}
	}
}

func TestClonarBaseNSSTemporal_AdmiteArchivosAuxiliaresAusentes(t *testing.T) {
	perfil := t.TempDir()
	if err := os.WriteFile(filepath.Join(perfil, "cert9.db"), []byte("certificados"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(perfil, "key4.db"), []byte("claves"), 0o600); err != nil {
		t.Fatal(err)
	}

	clon, err := clonarBaseNSSTemporal(perfil)
	if err != nil {
		t.Fatalf("clonar base NSS sin WAL, SHM ni pkcs11.txt: %v", err)
	}
	defer os.RemoveAll(clon)
	for _, nombre := range []string{"cert9.db", "key4.db"} {
		if _, err := os.Stat(filepath.Join(clon, nombre)); err != nil {
			t.Fatalf("falta %q en el clon: %v", nombre, err)
		}
	}
	for _, nombre := range []string{"cert9.db-wal", "cert9.db-shm", "key4.db-wal", "key4.db-shm", "pkcs11.txt"} {
		if _, err := os.Lstat(filepath.Join(clon, nombre)); !os.IsNotExist(err) {
			t.Fatalf("archivo auxiliar ausente %q creado en el clon: %v", nombre, err)
		}
	}
}
