// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"strings"
	"testing"
)

func TestLeyendaCSVValidatesLimitsAndQRChoice(t *testing.T) {
	options := map[string]string{"csv": "ABC-123", "csvUrl": "https://sede.example/consulta?csv={csv}", "csvQR": "false"}
	parsed, active, err := leerOpcionesLeyendaCSV(options)
	if err != nil || !active || parsed.qr || !strings.Contains(parsed.direccion, "ABC-123") {
		t.Fatalf("options = %+v, active = %t, err = %v", parsed, active, err)
	}
	options["csvUrl"] = "http://sede.example/consulta"
	if _, _, err := leerOpcionesLeyendaCSV(options); err == nil {
		t.Fatal("HTTP URL accepted")
	}
	options["csvUrl"] = "https://sede.example/consulta"
	options["csv"] = strings.Repeat("A", 129)
	if _, _, err := leerOpcionesLeyendaCSV(options); err == nil {
		t.Fatal("oversized CSV accepted")
	}
	options["csv"] = "ABC"
	options["csvText"] = strings.Repeat("X", 513)
	if _, _, err := leerOpcionesLeyendaCSV(options); err == nil {
		t.Fatal("oversized legend accepted")
	}
}
