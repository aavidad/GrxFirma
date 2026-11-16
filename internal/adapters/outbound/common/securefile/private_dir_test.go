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

func TestEnsurePrivateDirCreaYComprueba(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := CheckPrivateDir(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("carpeta inexistente: %v", err)
	}
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := CheckPrivateDir(dir); err != nil {
		t.Fatalf("carpeta recién creada no privada: %v", err)
	}
}

func TestEnsurePrivateDirRechazaRutaRelativa(t *testing.T) {
	for _, mala := range []string{"", "relativa", "a\x00b"} {
		if err := EnsurePrivateDir(mala); err == nil {
			t.Fatalf("ruta %q admitida", mala)
		}
	}
}

func TestEnsurePrivateDirRechazaEnlace(t *testing.T) {
	destino := t.TempDir()
	enlace := filepath.Join(t.TempDir(), "enlace")
	if err := os.Symlink(destino, enlace); err != nil {
		t.Skipf("no se pueden crear enlaces: %v", err)
	}
	if err := EnsurePrivateDir(enlace); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("enlace admitido al crear: %v", err)
	}
	if err := CheckPrivateDir(enlace); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("enlace admitido al comprobar: %v", err)
	}
}
