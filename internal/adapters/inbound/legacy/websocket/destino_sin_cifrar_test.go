// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
)

// Lote 13c (A-03): por el canal socket, una aplicación de intranet en http
// que la política autoriza vuelve a atenderse, y la operación llega marcada
// para que la aprobación avise de que el destino no usa conexión cifrada.
func TestHandleText_DestinoHTTPDeIntranetLlegaMarcado(t *testing.T) {
	politica := trustPolicyMock{decision: domain.TrustDecision{Status: domain.TrustAllowed}}
	for _, caso := range []struct {
		mensaje string
		marcado bool
	}{
		{"afirma://sign?id=1&rtservlet=http%3A%2F%2Fintranet.example%3A8080%2Frt&stservlet=http%3A%2F%2Fintranet.example%3A8080%2Fst", true},
		{"afirma://sign?id=1&rtservlet=https%3A%2F%2Fsede.example%2Frt&stservlet=https%3A%2F%2Fsede.example%2Fst", false},
	} {
		manejador := &manejadorCaptura{}
		adaptador := New(afirmauri.New(politica), nil).WithLegacyHandler(manejador)
		if _, err := adaptador.HandleText(context.Background(), "https://sede.example", caso.mensaje); err != nil {
			t.Fatalf("%s: debía atenderse: %v", caso.mensaje, err)
		}
		if manejador.llamadas.Load() != 1 || manejador.sinCifrar.Load() != caso.marcado {
			t.Fatalf("%s: llamadas=%d marca=%v, se esperaba %v", caso.mensaje, manejador.llamadas.Load(), manejador.sinCifrar.Load(), caso.marcado)
		}
	}

	denegada := trustPolicyMock{decision: domain.TrustDecision{Status: domain.TrustDenied}}
	manejador := &manejadorCaptura{}
	adaptador := New(afirmauri.New(denegada), nil).WithLegacyHandler(manejador)
	if _, err := adaptador.HandleText(context.Background(), "https://sede.example",
		"afirma://sign?id=1&rtservlet=http%3A%2F%2Fintranet.example%3A8080%2Frt"); err == nil {
		t.Fatal("sin autorización de la política el destino http debía rechazarse")
	}
	if manejador.llamadas.Load() != 0 {
		t.Fatal("una solicitud rechazada no debía llegar al manejador")
	}
}
