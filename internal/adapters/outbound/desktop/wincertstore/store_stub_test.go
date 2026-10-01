// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package wincertstore_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/domain"
)

// TestStub_List_SinError verifica que el stub compila y List() retorna sin error.
func TestStub_List_SinError(t *testing.T) {
	t.Parallel()

	almacen := wincertstore.New()
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("stub List() error inesperado = %v", err)
	}
	if refs != nil {
		t.Errorf("stub List() debería retornar nil, obtenido %v", refs)
	}
}

func TestStub_KeyFor_InformaPlataformaNoDisponible(t *testing.T) {
	t.Parallel()

	almacen := wincertstore.New()
	_, err := almacen.KeyFor(context.Background(), domain.CertificateRef{})
	if !errors.Is(err, wincertstore.ErrNoDisponibleEnEstaPlataforma) {
		t.Fatalf("KeyFor() error = %v, want ErrNoDisponibleEnEstaPlataforma", err)
	}
}
