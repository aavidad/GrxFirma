// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
)

func TestConstruirFuentesCertificados_IncluyeProveedorWindows(t *testing.T) {
	_, provider, _, err := construirFuentesCertificados(
		context.Background(),
		"",
		"",
		t.TempDir(),
		nil,
	)
	if err != nil {
		t.Fatalf("construirFuentesCertificados() error = %v", err)
	}

	aggregate, ok := provider.(*proveedorClavesAgregado)
	if !ok {
		t.Fatalf("proveedor = %T, want *proveedorClavesAgregado", provider)
	}
	found := 0
	for _, source := range aggregate.fuentes {
		if _, ok := source.(*wincertstore.Almacen); ok {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("proveedores Windows = %d, want 1", found)
	}
}

func TestErrorProveedorWindowsNoAplicable_NoOcultaOtrasFuentes(t *testing.T) {
	if !esErrorProveedorNoAplicable(
		wincertstore.ErrNoDisponibleEnEstaPlataforma,
	) {
		t.Fatal("el stub Windows debe tratarse como proveedor no aplicable")
	}
}
