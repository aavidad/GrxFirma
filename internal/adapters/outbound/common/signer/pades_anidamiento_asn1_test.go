// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/common/anidamientoasn1"
)

// E2-05: un CMS con millones de niveles "30 80" no debe llegar al
// decodificador recursivo (agotaría la pila, que es fatal en Go).
func TestNormalizeBERToDER_RechazaAnidamientoExcesivo(t *testing.T) {
	profundo := bytes.Repeat([]byte{0x30, 0x80}, 4<<20)
	_, err := normalizeBERToDER(profundo)
	if !errors.Is(err, anidamientoasn1.ErrDemasiadoProfundo) {
		t.Fatalf("se esperaba ErrDemasiadoProfundo, se obtuvo %v", err)
	}
}
