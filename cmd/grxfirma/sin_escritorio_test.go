// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/desktop/escritorio"
)

// Sin escritorio la aplicación de escritorio no se abre y el motivo sale del
// catálogo.
func TestSinEscritorioLaAplicacionDeEscritorioNoSeAbre(t *testing.T) {
	loc := localizador.Para("es")
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	var stderr bytes.Buffer
	if escritorioParaDesktop(&stderr, loc.T) {
		t.Fatal("sin escritorio se permitió abrir la aplicación")
	}
	if got := strings.TrimSpace(stderr.String()); got != loc.T(escritorio.ClaveMensaje) || got == escritorio.ClaveMensaje {
		t.Fatalf("mensaje sin escritorio = %q", got)
	}
	escritorio.SustituirDetector(func() bool { return true })
	if !escritorioParaDesktop(&stderr, loc.T) {
		t.Fatal("con escritorio no se permitió abrir la aplicación")
	}
}
