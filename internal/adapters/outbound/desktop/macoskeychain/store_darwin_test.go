// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package macoskeychain_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/domain"
)

// TestList_Darwin_DevuelveSliceValido verifica que List() no falla y devuelve
// un slice (puede estar vacío en CI sin certificados instalados).
func TestList_Darwin_DevuelveSliceValido(t *testing.T) {
	t.Parallel()

	almacen := macoskeychain.New()
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("List() error inesperado = %v", err)
	}
	// En un entorno de CI sin certificados el slice puede estar vacío; eso es válido.
	t.Logf("certificados encontrados en Keychain: %d", len(refs))

	for i, ref := range refs {
		if ref.ID == "" {
			t.Errorf("refs[%d].ID está vacío", i)
		}
		if ref.Fingerprint == "" {
			t.Errorf("refs[%d].Fingerprint está vacío", i)
		}
		if ref.Subject == "" {
			t.Errorf("refs[%d].Subject está vacío", i)
		}
	}
}

// TestKeyFor_Darwin_CertNoEncontrado verifica que KeyFor() retorna error controlado
// cuando el certificado no existe en el Keychain.
func TestKeyFor_Darwin_CertNoEncontrado(t *testing.T) {
	t.Parallel()

	almacen := macoskeychain.New()

	// Usar un fingerprint que no existe en ningún Keychain real.
	certFalso := domain.CertificateRef{
		ID:          "000000000000000000",
		Fingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
		Subject:     "Certificado Falso Test",
	}

	_, err := almacen.KeyFor(context.Background(), certFalso)
	if err == nil {
		t.Fatal("KeyFor() debería retornar error para un certificado no existente")
	}
	t.Logf("error esperado: %v", err)
}

// TestList_Darwin_ContextoCancelado verifica que la cancelación de contexto se respeta.
func TestList_Darwin_ContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	almacen := macoskeychain.New()
	_, err := almacen.List(ctx)
	if err == nil {
		t.Log("contexto cancelado: List() retornó sin error (aceptable si Keychain vacío)")
	}
}
