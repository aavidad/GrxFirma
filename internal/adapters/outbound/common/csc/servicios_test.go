// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import "testing"

func TestServicioPermitido(t *testing.T) {
	for _, c := range []struct {
		url   string
		lista []string
		want  bool
	}{
		{"https://firma.example.es/csc/v2", nil, true},
		{"https://firma.example.es/csc/v2", []string{}, false},
		{"https://firma.example.es/csc/v2", []string{"FIRMA.example.es"}, true},
		{"https://firma.example.es:443", []string{"firma.example.es"}, true},
		{"https://firma.example.es:8443", []string{"firma.example.es"}, false},
		{"https://firma.example.es:8443", []string{"firma.example.es:8443"}, true},
		{"https://firma.example.es.atacante.example", []string{"firma.example.es"}, false},
		{"https://firma.example.es", []string{"https://firma.example.es/", "firma.example.es/x"}, false},
		{"no es url", []string{"firma.example.es"}, false},
	} {
		if got := ServicioPermitido(c.url, c.lista); got != c.want {
			t.Errorf("ServicioPermitido(%q, %q) = %v, want %v", c.url, c.lista, got, c.want)
		}
	}
}
