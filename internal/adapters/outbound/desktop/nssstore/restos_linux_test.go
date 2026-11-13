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
	"time"
)

// F-03: los restos de exportaciones interrumpidas se borran, pero solo los
// propios, abandonados y sin enlaces.
func TestLimpiarRestosTemporales(t *testing.T) {
	t.Parallel()
	raiz := t.TempDir()
	base := filepath.Join(raiz, "temporal")
	tmp := filepath.Join(raiz, "tmp")
	for _, dir := range []string{base, tmp} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	escribir := func(ruta string) {
		t.Helper()
		if err := os.WriteFile(ruta, []byte("secreto"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	antiguo := time.Now().Add(-time.Hour)
	envejecer := func(ruta string) {
		t.Helper()
		if err := os.Chtimes(ruta, antiguo, antiguo); err != nil {
			t.Fatal(err)
		}
	}

	pw := filepath.Join(tmp, "grxfirma-openssl-pw-1")
	escribir(pw)
	envejecer(pw)
	p12 := filepath.Join(tmp, "grxfirma-nss-1.p12")
	escribir(p12)
	envejecer(p12)
	clon := filepath.Join(tmp, "grxfirma-nssdb-1")
	if err := os.Mkdir(clon, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(filepath.Join(clon, "key4.db"))
	envejecer(clon)
	trabajo := filepath.Join(base, "grxfirma-nss-1")
	if err := os.Mkdir(trabajo, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(filepath.Join(trabajo, "exportado.p12"))
	envejecer(trabajo)

	reciente := filepath.Join(tmp, "grxfirma-pw-reciente")
	escribir(reciente)
	ajeno := filepath.Join(raiz, "ajeno")
	escribir(ajeno)
	enlazado := filepath.Join(tmp, "grxfirma-pw-enlace")
	if err := os.Link(ajeno, enlazado); err != nil {
		t.Fatal(err)
	}
	envejecer(enlazado)
	simbolico := filepath.Join(tmp, "grxfirma-import-1.p12")
	if err := os.Symlink(ajeno, simbolico); err != nil {
		t.Fatal(err)
	}
	otro := filepath.Join(tmp, "otro-programa")
	escribir(otro)
	envejecer(otro)

	limpiarRestosTemporales(base, tmp, time.Now())

	for _, ruta := range []string{pw, p12, clon, trabajo} {
		if _, err := os.Lstat(ruta); !os.IsNotExist(err) {
			t.Errorf("resto abandonado sin borrar: %s (%v)", ruta, err)
		}
	}
	for _, ruta := range []string{reciente, enlazado, simbolico, otro} {
		if _, err := os.Lstat(ruta); err != nil {
			t.Errorf("se borró lo que no debía: %s (%v)", ruta, err)
		}
	}
	if contenido, err := os.ReadFile(ajeno); err != nil || string(contenido) != "secreto" {
		t.Fatalf("se sobrescribió un fichero ajeno a través de un enlace: %q, %v", contenido, err)
	}
}
