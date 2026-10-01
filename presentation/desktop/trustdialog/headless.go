// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog

import "context"

// HeadlessUI aplica siempre la misma decisión sin mostrar ningún diálogo.
// Útil para tests, modo CLI y entornos sin display.
type HeadlessUI struct {
	Decision DecisionUnicaVez
}

// PedirDecision retorna siempre la decisión configurada.
func (h *HeadlessUI) PedirDecision(_ context.Context, _ string) (DecisionUnicaVez, error) {
	return h.Decision, nil
}

var _ TrustUIProvider = (*HeadlessUI)(nil)
