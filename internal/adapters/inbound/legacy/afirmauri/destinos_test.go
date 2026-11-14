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

// A-03: en el camino directo los destinos son https, o http solo en
// loopback, aunque la política los admita (como ya exigía el orquestador antes
// del lote 4).
func TestParse_DestinoHTTPFueraDeLoopbackSeRechaza(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	for _, raw := range []string{
		"afirma://sign?id=1&rtservlet=http%3A%2F%2F10.0.0.5%2Fx",
		"afirma://batch?id=1&rtservlet=http%3A%2F%2Fintranet%3A8080%2Fx",
	} {
		if _, err := adaptador.Parse(context.Background(), raw); err == nil {
			t.Fatalf("Parse(%s) debía fallar por formato", raw)
		}
	}
	if _, err := adaptador.Parse(context.Background(), "afirma://sign?id=1&rtservlet=http%3A%2F%2F127.0.0.1%3A8080%2Fx"); err != nil {
		t.Fatalf("http en loopback debe seguir admitiéndose: %v", err)
	}
}

// Lote 13c (A-03): por el canal socket, un destino http de intranet que la
// política autoriza vuelve a admitirse, como antes del lote 4, y queda
// marcado para el aviso. Si la política no lo autoriza, se rechaza.
func TestParseSocket_DestinoHTTPDeIntranetAutorizadoConAviso(t *testing.T) {
	permitido := New(trustStub{estado: domain.TrustAllowed})
	for _, raw := range []string{
		"afirma://sign?id=1&rtservlet=http%3A%2F%2F10.0.0.5%2Fx&stservlet=http%3A%2F%2F10.0.0.5%2Fy",
		"afirma://batch?id=1&rtservlet=http%3A%2F%2Fintranet%3A8080%2Fx",
	} {
		solicitud, err := permitido.ParseSocket(context.Background(), raw)
		if err != nil {
			t.Fatalf("ParseSocket(%s) debía admitirse: %v", raw, err)
		}
		if !solicitud.DestinoSinCifrar {
			t.Fatalf("ParseSocket(%s): falta la marca de destino sin cifrar", raw)
		}
	}
	solicitud, err := permitido.ParseSocket(context.Background(), "afirma://sign?id=1&rtservlet=https%3A%2F%2Fsede.example%2Fx")
	if err != nil || solicitud.DestinoSinCifrar {
		t.Fatalf("https no lleva la marca: %v, %v", solicitud.DestinoSinCifrar, err)
	}
	denegado := New(trustStub{estado: domain.TrustDenied})
	if _, err := denegado.ParseSocket(context.Background(), "afirma://sign?id=1&rtservlet=http%3A%2F%2F10.0.0.5%2Fx"); err == nil {
		t.Fatal("un destino http que la política no autoriza debía rechazarse")
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
