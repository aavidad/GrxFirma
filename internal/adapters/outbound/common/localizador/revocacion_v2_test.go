// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador

import "testing"

func TestRevocacionNoConcluyenteTraducida(t *testing.T) {
	for _, idioma := range []string{"es", "en", "ca", "va", "gl", "eu", "fr", "it", "pt", "de", "zh"} {
		got := Para(idioma).T("revocación no concluyente")
		if got == "" || got == "revocación no concluyente" && idioma != "es" {
			t.Fatalf("falta traducción %s: %q", idioma, got)
		}
	}
}
