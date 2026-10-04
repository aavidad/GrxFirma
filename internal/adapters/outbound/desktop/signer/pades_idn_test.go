// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestURLsVerificacionIDN(t *testing.T) {
	data, err := os.ReadFile("../../../../../testdata/verification_urls.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Valid   []struct{ Input, Expected string }
		Invalid []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases.Valid {
		t.Run(tc.Input, func(t *testing.T) {
			got, err := normalizarURLQRSello(tc.Input)
			if err != nil || got != tc.Expected {
				t.Fatalf("QR=%q, esperado=%q, error=%v", got, tc.Expected, err)
			}
			options := map[string]string{"qrContent": tc.Input}
			if err := normalizarQROpciones(options); err != nil || options["qrContent"] != tc.Expected {
				t.Fatalf("opciones QR=%v, error=%v", options, err)
			}
			csv, active, err := leerOpcionesLeyendaCSV(map[string]string{"csv": "ABC-123", "csvUrl": tc.Input})
			wantCSV := strings.ReplaceAll(tc.Expected, "{csv}", "ABC-123")
			if err != nil || !active || csv.direccion != wantCSV || !strings.Contains(csv.texto, wantCSV) {
				t.Fatalf("CSV=%+v, esperado=%q, error=%v", csv, wantCSV, err)
			}
			again, err := normalizarURLQRSello(got)
			if err != nil || again != got {
				t.Fatalf("ACE no es estable: %q, error=%v", again, err)
			}
		})
	}
	for _, input := range cases.Invalid {
		t.Run(input, func(t *testing.T) {
			if got, err := normalizarURLQRSello(input); err == nil {
				t.Errorf("QR inválido aceptado: %q", got)
			}
			if got, _, err := leerOpcionesLeyendaCSV(map[string]string{"csv": "ABC", "csvUrl": input}); err == nil {
				t.Errorf("CSV inválido aceptado: %+v", got)
			}
		})
	}
}

func TestQRAliasUnicodeYAceCoinciden(t *testing.T) {
	options := map[string]string{
		"qrContent":            "https://café.es/verificar",
		"visibleSealQRContent": "https://xn--caf-dma.es/verificar",
	}
	if err := normalizarQROpciones(options); err != nil {
		t.Fatal(err)
	}
	if options["qrContent"] != "https://xn--caf-dma.es/verificar" || len(options) != 1 {
		t.Fatalf("opciones normalizadas: %v", options)
	}
}
