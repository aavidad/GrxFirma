// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"testing"

	"grxfirma/internal/domain"
)

// A-03: los destinos tienen el mismo formato en todos los caminos: https, o
// http solo en loopback, aunque la política los admita.
func TestParse_DestinoHTTPFueraDeLoopbackSeRechaza(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	for _, raw := range []string{
		"afirma://sign?id=1&rtservlet=http%3A%2F%2F10.0.0.5%2Fx",
		"afirma://batch?id=1&rtservlet=http%3A%2F%2Fintranet%3A8080%2Fx",
	} {
		if _, err := adaptador.ParseSocket(context.Background(), raw); err == nil {
			t.Fatalf("ParseSocket(%s) debía fallar por formato", raw)
		}
		if _, err := adaptador.Parse(context.Background(), raw); err == nil {
			t.Fatalf("Parse(%s) debía fallar por formato", raw)
		}
	}
	if _, err := adaptador.Parse(context.Background(), "afirma://sign?id=1&rtservlet=http%3A%2F%2F127.0.0.1%3A8080%2Fx"); err != nil {
		t.Fatalf("http en loopback debe seguir admitiéndose: %v", err)
	}
}

func TestOrigenesDestinoSolicitudIncluyeSubidaYServidor(t *testing.T) {
	origenes, err := OrigenesDestinoSolicitud(Solicitud{
		Sesion:  domain.ExchangeSession{UploadEndpoint: "https://st.example/a", RetrieveEndpoint: "https://rt.example/b"},
		Options: map[string]string{"serverUrl": "https://fire.example/ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(origenes) != 3 {
		t.Fatalf("origenes = %v", origenes)
	}
}
