// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build pkcs11_integration

// Tests de integración reales para el adaptador PKCS#11.
// Requieren hardware físico (DNIe, tarjeta criptográfica o token USB)
// y el módulo opensc-pkcs11.so instalado.
//
// Ejecutar con:
//
//	go test -tags pkcs11_integration ./internal/adapters/outbound/desktop/pkcs11store/...
package pkcs11store_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11store"
)

// TestIntegracion_Enumerate_ConHardware enumera los tokens disponibles con hardware real.
func TestIntegracion_Enumerate_ConHardware(t *testing.T) {
	almacen := pkcs11store.NewAutodetect()
	defer almacen.Cerrar()

	tokens, err := almacen.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate() error = %v", err)
	}

	t.Logf("Tokens encontrados: %d", len(tokens))
	for i, tok := range tokens {
		t.Logf("  [%d] ID=%s Label=%s", i, tok.ID, tok.Label)
	}

	if len(tokens) == 0 {
		t.Skip("no se encontraron tokens — conecta hardware e intenta de nuevo")
	}
}

// TestIntegracion_List_ConHardware lista los certificados disponibles con hardware real.
func TestIntegracion_List_ConHardware(t *testing.T) {
	almacen := pkcs11store.NewAutodetect()
	defer almacen.Cerrar()

	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	t.Logf("Certificados encontrados: %d", len(refs))
	for i, ref := range refs {
		t.Logf("  [%d] ID=%s Subject=%s Issuer=%s Expira=%s",
			i, ref.ID, ref.Subject, ref.Issuer, ref.NotAfter.Format("2006-01-02"))
	}

	if len(refs) == 0 {
		t.Skip("no se encontraron certificados — conecta hardware e intenta de nuevo")
	}
}
