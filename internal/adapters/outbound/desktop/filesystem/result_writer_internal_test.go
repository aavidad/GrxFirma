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

// Firmar tres veces seguidas el mismo documento proponiendo la ruta de la
// firma anterior debe seguir la serie (_001, _002) y no encadenar sufijos.
func TestGenerarRutaAlternativa_SigueLaSerie(t *testing.T) {
	dir := t.TempDir()
	crear := func(nombre string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	crear("contrato_firmado.pdf")

	casos := []struct {
		propuesta string
		quiere    string
	}{
		{"contrato_firmado.pdf", "contrato_firmado_001.pdf"},
		{"contrato_firmado_001.pdf", "contrato_firmado_002.pdf"},
		{"contrato_firmado_002.pdf", "contrato_firmado_003.pdf"},
		// La serie salta los huecos ocupados.
		{"contrato_firmado_001.pdf", "contrato_firmado_004.pdf"},
	}
	for _, caso := range casos {
		crear(caso.propuesta)
		got, err := generarRutaAlternativa(filepath.Join(dir, caso.propuesta))
		if err != nil {
			t.Fatalf("%s: %v", caso.propuesta, err)
		}
		if filepath.Base(got) != caso.quiere {
			t.Fatalf("%s -> %s, quiere %s", caso.propuesta, filepath.Base(got), caso.quiere)
		}
		crear(caso.quiere)
	}
}

// Solo cuentan los sufijos de tres cifras: un año o un número de factura
// forma parte del nombre.
func TestGenerarRutaAlternativa_NoConfundeNumerosDelNombre(t *testing.T) {
	dir := t.TempDir()
	for _, caso := range []struct{ nombre, quiere string }{
		{"factura_2024.pdf", "factura_2024_001.pdf"},
		{"acta_12.pdf", "acta_12_001.pdf"},
		{"_001.pdf", "_001_001.pdf"},
		{"lote_999.pdf", "lote_999_001.pdf"},
	} {
		ruta := filepath.Join(dir, caso.nombre)
		if err := os.WriteFile(ruta, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := generarRutaAlternativa(ruta)
		if err != nil {
			t.Fatalf("%s: %v", caso.nombre, err)
		}
		if filepath.Base(got) != caso.quiere {
			t.Fatalf("%s -> %s, quiere %s", caso.nombre, filepath.Base(got), caso.quiere)
		}
	}
}
