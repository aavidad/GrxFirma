// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build tsa_integration

package tsaclient

import (
	"context"
	"crypto"
	"crypto/sha256"
	"testing"
)

func TestClient_RequestTimestamp_FreeTSA(t *testing.T) {
	client := New("https://freetsa.org/tsr")
	hash := sha256.Sum256([]byte("grxfirma tsa integration"))

	tst, err := client.RequestTimestamp(context.Background(), hash[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("RequestTimestamp() error = %v", err)
	}
	if len(tst) == 0 {
		t.Fatal("se esperaba TimeStampToken no vacio")
	}
}
