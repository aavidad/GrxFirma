// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package progressdialog_test

import (
	"context"
	"testing"

	"grxfirma/presentation/desktop/progressdialog"
)

func TestHeadlessReporter_Registra(t *testing.T) {
	t.Parallel()
	p := &progressdialog.HeadlessProvider{}
	r := p.MostrarProgreso(context.Background(), "Firmando…")

	r.SetMensaje("paso 1")
	r.SetProgreso(0.5)
	r.SetMensaje("paso 2")
	r.Cerrar()

	hr := p.Last
	if len(hr.Mensajes) != 2 {
		t.Errorf("esperados 2 mensajes, obtenidos %d", len(hr.Mensajes))
	}
	if hr.Mensajes[0] != "paso 1" {
		t.Errorf("mensaje[0] = %q", hr.Mensajes[0])
	}
	if hr.Progresos[0] != 0.5 {
		t.Errorf("progreso[0] = %v", hr.Progresos[0])
	}
	if !hr.Closed {
		t.Error("Cerrar() no marcó el reporter como cerrado")
	}
}

func TestHeadlessProvider_CadaLlamadaCreaReporterNuevo(t *testing.T) {
	t.Parallel()
	p := &progressdialog.HeadlessProvider{}

	r1 := p.MostrarProgreso(context.Background(), "op1")
	r1.SetMensaje("a")
	r2 := p.MostrarProgreso(context.Background(), "op2")
	r2.SetMensaje("b")

	if p.Last == nil {
		t.Fatal("Last no asignado")
	}
	if len(p.Last.Mensajes) != 1 || p.Last.Mensajes[0] != "b" {
		t.Errorf("Last debe ser el reporter de r2, obtenido %v", p.Last.Mensajes)
	}
}
