// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import "testing"

func TestIdiomaSelloCLI(t *testing.T) {
	if got := construirOpcionesFirmaCompat(configCLI{idiomaSello: "en"}, "PAdES")["sealLanguage"]; got != "en" {
		t.Fatalf("-idioma-sello no llega al motor: %q", got)
	}
	// -opcion sealLanguage=... explícita prevalece sobre la bandera.
	cfg := configCLI{idiomaSello: "en", opciones: map[string]string{"sealLanguage": "es"}}
	if got := construirOpcionesFirmaCompat(cfg, "PAdES")["sealLanguage"]; got != "es" {
		t.Fatalf("la opción explícita debe prevalecer: %q", got)
	}
	if _, ok := construirOpcionesFirmaCompat(configCLI{idiomaSello: "en"}, "CAdES")["sealLanguage"]; ok {
		t.Fatal("sin sello visible no se envía el idioma del sello")
	}
}
