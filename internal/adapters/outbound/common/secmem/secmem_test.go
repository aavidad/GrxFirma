// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secmem_test

import (
	"bytes"
	"testing"
	"unsafe"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

func TestBlob_Roundtrip(t *testing.T) {
	t.Parallel()
	src := []byte("clave-secreta-1234")
	b := secmem.New(src)
	if !bytes.Equal(b.Bytes(), src) {
		t.Fatalf("contenido no coincide: %v", b.Bytes())
	}
}

func TestBlob_NoComparteBacking(t *testing.T) {
	t.Parallel()
	src := []byte("original")
	b := secmem.New(src)
	src[0] = 'X' // modificar el original no debe afectar al Blob
	if b.Bytes()[0] == 'X' {
		t.Error("Blob comparte backing array con src")
	}
}

func TestBlob_Destroy_Zeroiza(t *testing.T) {
	t.Parallel()
	src := []byte{0x01, 0x02, 0x03, 0x04}
	b := secmem.New(src)
	copia := make([]byte, b.Len())
	copy(copia, b.Bytes())

	b.Destroy()

	if b.Len() != 0 {
		t.Errorf("Len() tras Destroy() = %d, esperado 0", b.Len())
	}
	// El contenido original sigue en copia pero b ya no lo expone.
	if bytes.Equal(b.Bytes(), copia) {
		t.Error("Bytes() tras Destroy() todavía retorna datos")
	}
}

func TestBlob_DestroyIdempotente(t *testing.T) {
	t.Parallel()
	b := secmem.New([]byte("dato"))
	b.Destroy()
	b.Destroy() // no debe entrar en pánico
}

func TestZeroize(t *testing.T) {
	t.Parallel()
	buf := []byte{1, 2, 3, 4, 5}
	secmem.Zeroize(buf)
	for i, v := range buf {
		if v != 0 {
			t.Errorf("buf[%d] = %d, esperado 0", i, v)
		}
	}
}

func TestUseReadOnlyString_NoCopiaYBorraElBuffer(t *testing.T) {
	t.Parallel()
	secret := []byte("secreto-efimero")
	var borrowed string

	err := secmem.UseReadOnlyString(secret, func(value string) error {
		borrowed = value
		if unsafe.StringData(value) != &secret[0] {
			t.Fatal("la vista sensible creó una copia inmutable")
		}
		if value != "secreto-efimero" {
			t.Fatalf("valor prestado = %q", value)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("UseReadOnlyString() error = %v", err)
	}
	for i, value := range secret {
		if value != 0 {
			t.Fatalf("byte %d no borrado: %d", i, value)
		}
	}
	for i := range borrowed {
		if borrowed[i] != 0 {
			t.Fatalf("la vista retenida conserva el byte %d", i)
		}
	}
}

func TestWithBlob_ZeorizaAlSalir(t *testing.T) {
	t.Parallel()
	src := []byte{0xAA, 0xBB, 0xCC}
	var capturado []byte
	secmem.WithBlob(src, func(b []byte) {
		capturado = b
		if !bytes.Equal(b, src) {
			t.Errorf("WithBlob: contenido no coincide: %v", b)
		}
	})
	// Después de salir, el buffer interno debe estar zerizado.
	for i, v := range capturado {
		if v != 0 {
			t.Errorf("capturado[%d] = %02X, esperado 0x00", i, v)
		}
	}
}

func TestWithBlob_ZeorizaInclusivoEnPanico(t *testing.T) {
	t.Parallel()
	src := []byte{0x01}
	var capturado []byte
	func() {
		defer func() { recover() }()
		secmem.WithBlob(src, func(b []byte) {
			capturado = b
			panic("panic de prueba")
		})
	}()
	if capturado != nil && capturado[0] != 0 {
		t.Errorf("buffer no zerizado tras pánico: %v", capturado)
	}
}

// TestBlobMlockBestEffort verifica que el fijado en RAM es best-effort: crear
// y destruir un Blob funciona tanto si mlock fue concedido como si no, y
// Locked() es coherente tras Destroy.
func TestBlobMlockBestEffort(t *testing.T) {
	b := secmem.New([]byte("material sensible"))
	t.Logf("mlock concedido: %v (false no es error; depende de RLIMIT_MEMLOCK)", b.Locked())
	if b.Len() == 0 {
		t.Fatal("el Blob debe contener el material")
	}
	b.Destroy()
	if b.Locked() {
		t.Error("tras Destroy el Blob no debe seguir marcado como fijado")
	}
	b.Destroy() // idempotente también con mlock
}
