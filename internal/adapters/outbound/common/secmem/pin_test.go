// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secmem_test

import (
	"bytes"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

func TestPIN_Roundtrip(t *testing.T) {
	t.Parallel()
	p, err := secmem.NuevoPIN([]byte("1234"))
	if err != nil {
		t.Fatalf("NuevoPIN error: %v", err)
	}
	defer p.Destroy()
	if !bytes.Equal(p.Bytes(), []byte("1234")) {
		t.Errorf("contenido no coincide: %v", p.Bytes())
	}
}

func TestPIN_Vacio_RetornaError(t *testing.T) {
	t.Parallel()
	_, err := secmem.NuevoPIN(nil)
	if !errors.Is(err, secmem.ErrPINVacio) {
		t.Errorf("esperado ErrPINVacio, obtenido: %v", err)
	}
	_, err = secmem.NuevoPIN([]byte{})
	if !errors.Is(err, secmem.ErrPINVacio) {
		t.Errorf("esperado ErrPINVacio para slice vacío, obtenido: %v", err)
	}
}

func TestPIN_Destroy_Zeriza(t *testing.T) {
	t.Parallel()
	p, _ := secmem.NuevoPIN([]byte{0xAA, 0xBB})
	p.Destroy()
	if p.Len() != 0 {
		t.Errorf("Len() tras Destroy() = %d", p.Len())
	}
}

func TestPIN_DestroyIdempotente(t *testing.T) {
	t.Parallel()
	p, _ := secmem.NuevoPIN([]byte("abc"))
	p.Destroy()
	p.Destroy() // no debe entrar en pánico
}

func TestPIN_DesdeString(t *testing.T) {
	t.Parallel()
	p, err := secmem.NuevoPINDesdeString("mipin")
	if err != nil {
		t.Fatalf("NuevoPINDesdeString error: %v", err)
	}
	defer p.Destroy()
	if !bytes.Equal(p.Bytes(), []byte("mipin")) {
		t.Errorf("contenido no coincide: %v", p.Bytes())
	}
}

func TestPIN_DesdeString_Vacio(t *testing.T) {
	t.Parallel()
	_, err := secmem.NuevoPINDesdeString("")
	if !errors.Is(err, secmem.ErrPINVacio) {
		t.Errorf("esperado ErrPINVacio, obtenido: %v", err)
	}
}

func TestPIN_Use_ZeorizaAlSalir(t *testing.T) {
	t.Parallel()
	p, _ := secmem.NuevoPIN([]byte{0xDE, 0xAD})
	var capturado []byte
	p.Use(func(b []byte) {
		capturado = b
	})
	// Tras Use(), la copia interna debe estar zerizada.
	for i, v := range capturado {
		if v != 0 {
			t.Errorf("capturado[%d] = 0x%02X, esperado 0x00", i, v)
		}
	}
	// El Blob original debe seguir intacto.
	if !bytes.Equal(p.Bytes(), []byte{0xDE, 0xAD}) {
		t.Errorf("Blob original modificado: %v", p.Bytes())
	}
}
