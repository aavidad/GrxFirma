// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// politicaSoloValide solo autoriza https://valide.redsara.es y https://fire.example.
type politicaSoloValide struct{ trustStub }

func (politicaSoloValide) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	if origin == "https://valide.redsara.es" || origin == "https://fire.example" {
		return domain.TrustDecision{Origin: origin, Status: domain.TrustAllowed}, nil
	}
	return domain.TrustDecision{Origin: origin, Status: domain.TrustDenied}, nil
}

func uriTrifasicaConServidor(servidorKey, servidor string) string {
	props := base64.StdEncoding.EncodeToString([]byte(servidorKey + "=" + servidor + "\n"))
	q := url.Values{}
	q.Set("format", "CAdEStri")
	q.Set("id", "X")
	q.Set("stservlet", "https://valide.redsara.es/a")
	q.Set("rtservlet", "https://valide.redsara.es/b")
	q.Set("dat", base64.StdEncoding.EncodeToString([]byte("documento")))
	q.Set("properties", props)
	return "afirma://sign?" + q.Encode()
}

// A-01: serverUrl recibe el documento y decide qué firma la clave; tiene que
// pasar por la misma política de confianza que los servlets.
func TestParse_ServerURLNoAutorizadoSeRechaza(t *testing.T) {
	adaptador := New(politicaSoloValide{})
	for _, clave := range []string{"serverUrl", "serverurl", "SERVER_URL", "ServerURL"} {
		raw := uriTrifasicaConServidor(clave, "https://evil.example/ts")
		if _, err := adaptador.Parse(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "no esta autorizado") {
			t.Fatalf("Parse con %s: err = %v; se esperaba 'no esta autorizado'", clave, err)
		}
		if _, err := adaptador.ParseSocket(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "no esta autorizado") {
			t.Fatalf("ParseSocket con %s: err = %v; se esperaba 'no esta autorizado'", clave, err)
		}
	}
}

func TestParse_ServerURLAutorizadoSeAnadeALosOrigenes(t *testing.T) {
	adaptador := New(politicaSoloValide{})
	solicitud, err := adaptador.Parse(context.Background(), uriTrifasicaConServidor("serverUrl", "https://fire.example/triphaseSignService"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	encontrado := false
	for _, origen := range solicitud.Origenes {
		if origen == "https://fire.example" {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatalf("origenes = %v; falta el servidor trifásico", solicitud.Origenes)
	}
}

// Un servidor sin esquema o en http no se admite como destino trifásico.
func TestParse_ServerURLMalFormadoSeRechaza(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	for _, servidor := range []string{"evil.example/ts", "http://evil.example/ts"} {
		if _, err := adaptador.ParseSocket(context.Background(), uriTrifasicaConServidor("serverUrl", servidor)); err == nil {
			t.Fatalf("ParseSocket con serverUrl=%q debía fallar", servidor)
		}
	}
}
