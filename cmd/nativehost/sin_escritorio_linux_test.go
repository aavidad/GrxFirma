// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
)

// Sin DISPLAY ni WAYLAND_DISPLAY el selector de identidad no lanza ninguna
// herramienta y devuelve el error tipado.
func TestSinEscritorioElSelectorDeIdentidadNoSeLanza(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	t.Setenv("PATH", t.TempDir())
	if _, err := seleccionarIndiceCertificadoSistema(context.Background(), "t", "p", "u", "c", []string{"a"}); !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("selector de identidad sin escritorio = %v", err)
	}
}
