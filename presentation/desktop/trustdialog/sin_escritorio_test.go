// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/trustdialog"
)

// Sin escritorio el diálogo Fyne no se abre: el origen desconocido queda
// rechazado con el error tipado y nunca se confía en él por defecto.
func TestSinEscritorioElOrigenDesconocidoNoSeConfia(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	base := newMock(domain.TrustPending)
	policy := trustdialog.New(base, trustdialog.NewFyneTrustDialog(nil))

	decision, err := policy.Evaluate(context.Background(), "https://sede.example")
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("Evaluate sin escritorio = %v", err)
	}
	if decision.Status == domain.TrustAllowed || base.allowed["https://sede.example"] {
		t.Fatalf("sin escritorio se confió en el origen: %+v", decision)
	}
}
