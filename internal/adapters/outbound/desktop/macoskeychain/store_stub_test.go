// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !darwin || !cgo

package macoskeychain_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/domain"
)

// TestStub_List_SinError verifica que el stub compila y List() retorna sin error.
func TestStub_List_SinError(t *testing.T) {
	t.Parallel()

	almacen := macoskeychain.New()
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("stub List() error inesperado = %v", err)
	}
	if refs != nil {
		t.Errorf("stub List() debería retornar nil, obtenido %v", refs)
	}
}

// TestStub_KeyFor_ErrorControlado verifica que KeyFor() retorna el error esperado
// y no produce panic en plataformas no-macOS.
func TestStub_KeyFor_ErrorControlado(t *testing.T) {
	t.Parallel()

	almacen := macoskeychain.New()
	cert := domain.CertificateRef{
		ID:          "abc123",
		Fingerprint: "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
		Subject:     "Test",
	}

	key, err := almacen.KeyFor(context.Background(), cert)
	if err == nil {
		t.Fatal("stub KeyFor() debe retornar error en plataforma no-macOS")
	}
	if !errors.Is(err, macoskeychain.ErrNoDisponibleEnEstaPlataforma) {
		t.Errorf("error esperado ErrNoDisponibleEnEstaPlataforma, obtenido: %v", err)
	}
	if key != nil {
		t.Error("stub KeyFor() debe retornar nil como clave")
	}
}
