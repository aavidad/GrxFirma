// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
)

func TestConstruirFuentesCertificadosIncluyeClaveWindows(t *testing.T) {
	t.Parallel()

	_, proveedor, err := construirFuentesCertificados(t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("construirFuentesCertificados() error = %v", err)
	}
	agregado, ok := proveedor.(*proveedorClavesAgregado)
	if !ok {
		t.Fatalf("proveedor = %T, se esperaba *proveedorClavesAgregado", proveedor)
	}

	for _, fuente := range agregado.fuentes {
		if _, ok := fuente.(*wincertstore.Almacen); ok {
			return
		}
	}
	t.Fatal("el catálogo Windows se añadió sin su proveedor de clave privada")
}
