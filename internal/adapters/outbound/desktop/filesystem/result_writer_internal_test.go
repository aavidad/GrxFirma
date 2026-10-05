// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// crearDestinoExclusivo es el respaldo para sistemas de ficheros sin enlaces
// duros (FAT/exFAT): tampoco puede reemplazar un fichero existente.
func TestCrearDestinoExclusivo_NoReemplaza(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "doc.pdf")
	if err := crearDestinoExclusivo(ruta, 0o600, []byte("primero")); err != nil {
		t.Fatalf("primera creación: %v", err)
	}
	if err := crearDestinoExclusivo(ruta, 0o600, []byte("segundo")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("segunda creación = %v, want ErrExist", err)
	}
	if got, _ := os.ReadFile(ruta); string(got) != "primero" {
		t.Fatalf("contenido = %q", got)
	}
}
