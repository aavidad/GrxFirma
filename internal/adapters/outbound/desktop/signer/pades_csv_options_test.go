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

func TestLeyendaCSVRechazaControlYFormato(t *testing.T) {
	invisibles := []string{"\u202e", "\u202a", "\u2066", "\u2069", "\u200b", "\u200d", "\u200f", "\ufeff", "\u00ad", "\x1b", "\x00", "\u0085", "\U000E0001"}
	for _, r := range invisibles {
		base := map[string]string{"csv": "ABC-123", "csvUrl": "https://sede.example/consulta?csv={csv}"}
		codigo := map[string]string{"csv": "ABC" + r + "123", "csvUrl": base["csvUrl"]}
		if _, _, err := leerOpcionesLeyendaCSV(codigo); err == nil {
			t.Errorf("código CSV con %q aceptado", r)
		}
		texto := map[string]string{"csv": "ABC-123", "csvUrl": base["csvUrl"], "csvText": "Cotejo " + r + "{url}"}
		if _, _, err := leerOpcionesLeyendaCSV(texto); err == nil {
			t.Errorf("texto CSV con %q aceptado", r)
		}
		direccion := map[string]string{"csv": "ABC-123", "csvUrl": "https://sede.example/con" + r + "sulta"}
		if _, _, err := leerOpcionesLeyendaCSV(direccion); err == nil {
			t.Errorf("URL CSV con %q aceptada", r)
		}
		if got, err := normalizarURLQRSello("https://sede.example/" + r + "x"); err == nil {
			t.Errorf("URL QR con %q aceptada: %q", r, got)
		}
		if _, _, err := leerOpcionesLeyendaCSV(base); err != nil {
			t.Fatalf("leyenda válida rechazada: %v", err)
		}
	}
}
