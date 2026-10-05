package pkcs7

import (
	"bytes"
	"testing"
)

// AutoFirmaV2: una cadena de SEQUENCE de longitud indefinida anidadas sin
// fin recursaba hasta agotar la pila al verificar la firma de un PDF.
func TestBer2derAnidamientoExcesivo(t *testing.T) {
	const n = 200000
	ber := append(bytes.Repeat([]byte{0x30, 0x80}, n), bytes.Repeat([]byte{0x00, 0x00}, n)...)
	if _, err := ber2der(ber); err == nil {
		t.Fatal("se esperaba un error por anidamiento excesivo")
	}
	// Anidamiento normal.
	normal := append(bytes.Repeat([]byte{0x30, 0x80}, 20), 0x02, 0x01, 0x05)
	normal = append(normal, bytes.Repeat([]byte{0x00, 0x00}, 20)...)
	if _, err := ber2der(normal); err != nil {
		t.Fatalf("anidamiento de 20 niveles: %v", err)
	}
}
