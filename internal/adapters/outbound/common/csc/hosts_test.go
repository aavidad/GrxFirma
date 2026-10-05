// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"net/url"
	"testing"
)

func TestOAuthPermitidoMismoHostOParConfigurado(t *testing.T) {
	u := func(s string) *url.URL {
		v, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	pares, err := ParsearParesOAuth([]string{"Firma.Prestador.example=auth.prestador.example:8443"})
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		servicio, oauth string
		pares           []ParOAuth
		permitido       bool
	}{
		{"https://firma.prestador.example/csc/v2", "https://FIRMA.prestador.example:443/", nil, true},
		{"https://firma.prestador.example/csc/v2", "https://auth.prestador.example:8443", nil, false},
		{"https://firma.prestador.example/csc/v2", "https://auth.prestador.example:8443", pares, true},
		{"https://firma.prestador.example/csc/v2", "https://auth.prestador.example", pares, false},
		{"https://otro.example/csc/v2", "https://auth.prestador.example:8443", pares, false},
		{"https://firma.prestador.example:8443/csc/v2", "https://firma.prestador.example", nil, false},
	}
	for _, c := range casos {
		if got := oauthPermitido(u(c.servicio), u(c.oauth), c.pares); got != c.permitido {
			t.Errorf("%s -> %s (%v): %v, se esperaba %v", c.servicio, c.oauth, c.pares, got, c.permitido)
		}
	}
}

func TestParsearParesOAuthRechazaEntradasMalFormadas(t *testing.T) {
	for _, mala := range []string{"sin-igual", "=auth.example", "firma.example=", "https://firma.example=auth.example", "firma.example=auth.example/ruta", "a@firma.example=auth.example"} {
		if _, err := ParsearParesOAuth([]string{"bien.example=auth.example", mala}); CodigoDe(err) != CodigoParametroInvalido {
			t.Errorf("%q debió rechazarse: %v", mala, err)
		}
	}
}
