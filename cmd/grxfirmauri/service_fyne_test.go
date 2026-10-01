// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
)

type aprobacionWebFyneStub struct {
	approved bool
	err      error
	calls    int
}

func (a *aprobacionWebFyneStub) Request(context.Context, string) (bool, error) {
	a.calls++
	return a.approved, a.err
}

func TestRenderEstadoWebSocketActivo_MuestraEstadoYDireccion(t *testing.T) {
	texto := renderEstadoWebSocketActivo("127.0.0.1:8083")

	for _, esperado := range []string{
		"SERVIDOR WEBSOCKET ACTIVO",
		"**Estado:** ACTIVO",
		"`wss://127.0.0.1:8083`",
		"Detener servidor",
	} {
		if !strings.Contains(texto, esperado) {
			t.Fatalf("el estado visible no contiene %q:\n%s", esperado, texto)
		}
	}
}

func TestRenderEstadoWebSocketActivo_UsaDireccionPorDefecto(t *testing.T) {
	texto := renderEstadoWebSocketActivo(" ")
	if !strings.Contains(texto, "`wss://127.0.0.1:8080`") {
		t.Fatalf("se esperaba dirección por defecto en el estado visible:\n%s", texto)
	}
}

func TestSolicitarAprobacionFirmaWeb_RechazoBloqueaOperacionesDeFirma(t *testing.T) {
	t.Parallel()

	for _, operation := range []afirmauri.TipoOperacion{
		afirmauri.OperacionFirma,
		afirmauri.OperacionLote,
		afirmauri.OperacionSignSave,
	} {
		operation := operation
		t.Run(string(operation), func(t *testing.T) {
			t.Parallel()
			approval := &aprobacionWebFyneStub{approved: false}
			err := solicitarAprobacionFirmaWeb(
				context.Background(),
				afirmauri.Solicitud{Operacion: operation},
				approval,
			)
			if err == nil || !strings.Contains(err.Error(), "cancelado") {
				t.Fatalf("solicitarAprobacionFirmaWeb() error = %v, want cancellation", err)
			}
			if approval.calls != 1 {
				t.Fatalf("approval.calls = %d, want 1", approval.calls)
			}
		})
	}
}

func TestSolicitarAprobacionFirmaWeb_SinAprobadorFallaCerrado(t *testing.T) {
	t.Parallel()

	err := solicitarAprobacionFirmaWeb(
		context.Background(),
		afirmauri.Solicitud{Operacion: afirmauri.OperacionFirma},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "no configurado") {
		t.Fatalf("solicitarAprobacionFirmaWeb() error = %v, want fail-closed", err)
	}
}

func TestSolicitarAprobacionFirmaWeb_AprobacionPositivaSeSolicitaUnaVez(t *testing.T) {
	t.Parallel()

	approval := &aprobacionWebFyneStub{approved: true}
	if err := solicitarAprobacionFirmaWeb(
		context.Background(),
		afirmauri.Solicitud{Operacion: afirmauri.OperacionFirma},
		approval,
	); err != nil {
		t.Fatalf("solicitarAprobacionFirmaWeb() error = %v", err)
	}
	if approval.calls != 1 {
		t.Fatalf("approval.calls = %d, want exactly 1", approval.calls)
	}
}

func TestSolicitarAprobacionFirmaWeb_SelectCertNoFirma(t *testing.T) {
	t.Parallel()

	approval := &aprobacionWebFyneStub{approved: false}
	if err := solicitarAprobacionFirmaWeb(
		context.Background(),
		afirmauri.Solicitud{Operacion: afirmauri.OperacionSelectCert},
		approval,
	); err != nil {
		t.Fatalf("solicitarAprobacionFirmaWeb() error = %v", err)
	}
	if approval.calls != 0 {
		t.Fatalf("approval.calls = %d, want 0", approval.calls)
	}
}
