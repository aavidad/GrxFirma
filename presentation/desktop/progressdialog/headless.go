// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package progressdialog

import "context"

// HeadlessReporter descarta todas las actualizaciones sin mostrar UI.
// Útil para tests, modo CLI y entornos sin display.
type HeadlessReporter struct {
	Mensajes  []string
	Progresos []float64
	Closed    bool
}

func (h *HeadlessReporter) SetMensaje(msg string) { h.Mensajes = append(h.Mensajes, msg) }
func (h *HeadlessReporter) SetProgreso(f float64) { h.Progresos = append(h.Progresos, f) }
func (h *HeadlessReporter) Cerrar()               { h.Closed = true }

var _ Reporter = (*HeadlessReporter)(nil)

// HeadlessProvider crea HeadlessReporter sin mostrar ningún diálogo.
type HeadlessProvider struct {
	Last *HeadlessReporter
}

func (p *HeadlessProvider) MostrarProgreso(_ context.Context, _ string) Reporter {
	r := &HeadlessReporter{}
	p.Last = r
	return r
}

var _ ProgressProvider = (*HeadlessProvider)(nil)
