// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package documentpicker

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
)

// Sin escritorio el selector de documentos no abre ventana ni elige nada.
func TestSinEscritorioElSelectorDeDocumentoNoEspera(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	inicio := time.Now()
	documento, err := NewFyne().Pick(context.Background())
	if !errors.Is(err, escritorio.ErrSinEscritorio) || len(documento.Content) != 0 {
		t.Fatalf("Pick sin escritorio: %d bytes, err=%v", len(documento.Content), err)
	}
	if time.Since(inicio) > time.Second {
		t.Fatal("el selector esperó sin escritorio")
	}
}
