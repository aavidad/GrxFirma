// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"testing"
	"time"
)

func TestLeerFechaPDF(t *testing.T) {
	casos := map[string]time.Time{
		"D:20261005140402+02'00'": time.Date(2026, 10, 5, 12, 4, 2, 0, time.UTC),
		"D:20261005120402Z":       time.Date(2026, 10, 5, 12, 4, 2, 0, time.UTC),
		"D:20261005090402-03'00":  time.Date(2026, 10, 5, 12, 4, 2, 0, time.UTC),
		"D:20261005140402":        {}, // sin zona: ambigua
		"D:20261305140402Z":       {}, // mes 13
		"20261005140402Z":         {},
	}
	for valor, esperado := range casos {
		if got := leerFechaPDF(valor); !got.Equal(esperado) {
			t.Errorf("%q: %v, se esperaba %v", valor, got, esperado)
		}
	}
}

// /M solo cuenta si está dentro de lo firmado; /MDP y similares no son /M.
func TestExtractPDFSigningTimeM(t *testing.T) {
	bloque := []byte("<< /Type /Sig /MDP 2 /M (D:20261005140402+02'00') /Contents <00> >>")
	todo := [4]int{0, 1000, 1000, 0}
	if got := extractPDFSigningTimeM(bloque, 0, todo); !got.Equal(time.Date(2026, 10, 5, 12, 4, 2, 0, time.UTC)) {
		t.Fatalf("fecha = %v", got)
	}
	fuera := [4]int{0, 10, 2000, 10}
	if got := extractPDFSigningTimeM(bloque, 0, fuera); !got.IsZero() {
		t.Fatalf("una /M fuera del rango firmado no da fecha: %v", got)
	}
	if got := extractPDFSigningTimeM([]byte("<< /Type /Sig /Contents <00> >>"), 0, todo); !got.IsZero() {
		t.Fatalf("sin /M no hay fecha: %v", got)
	}
}
