// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"strings"
	"testing"

	"grxfirma/presentation/desktop/certpicker"
)

// A-02: el selector nativo de Windows muestra el mismo contexto que el de Fyne.
func TestAnteponerContextoSelectorMuestraDestino(t *testing.T) {
	ctx := certpicker.ConContextoDestino(context.Background(), "https://a.gob.es", "sign")
	if got := anteponerContextoSelector(ctx, "TEXTO"); !strings.Contains(got, "https://a.gob.es") || !strings.HasSuffix(got, "TEXTO") {
		t.Fatalf("contenido = %q", got)
	}
	if got := anteponerContextoSelector(context.Background(), "TEXTO"); got != "TEXTO" {
		t.Fatalf("sin contexto = %q", got)
	}
}
