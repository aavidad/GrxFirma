// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tsaclient

import (
	"bytes"
	"crypto"
	"errors"
	"math/big"
	"testing"

	"grxfirma/internal/adapters/outbound/common/anidamientoasn1"
)

// E2-05: el TimeStampToken de la respuesta de la TSA llega a timestamp.Parse,
// que lo pasa por ber2der de digitorus/pkcs7, recursivo y sin límite de
// profundidad. Ejecutar con -timeout 60s y GOMEMLIMIT=2GiB.
func TestValidateTimestampToken_AnidamientoProfundo(t *testing.T) {
	token := bytes.Repeat([]byte{0x30, 0x80}, 2<<20) // 4 MiB
	err := validateTimestampToken(token, make([]byte, 32), crypto.SHA256, big.NewInt(1))
	if !errors.Is(err, anidamientoasn1.ErrDemasiadoProfundo) {
		t.Fatalf("el anidamiento profundo no se cortó antes de timestamp.Parse: %v", err)
	}
}
