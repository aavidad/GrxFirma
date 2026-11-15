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

// Sin escritorio la firma no se aprueba y no se lanza zenity, kdialog,
// osascript ni el TaskDialog: la operación sale denegada al momento.
func TestSinEscritorioLaAprobacionNativaSaleDenegada(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	t.Setenv("PATH", t.TempDir())
	aprobado, err := (&aprobacionSistema{}).Request(context.Background(), "Firmar documento")
	if aprobado || !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("aprobación sin escritorio: aprobado=%v err=%v", aprobado, err)
	}
}
