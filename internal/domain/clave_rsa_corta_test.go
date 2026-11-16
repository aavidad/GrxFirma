// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "testing"

func TestClaveRSACortaSinExcepcion(t *testing.T) {
	forzada := VerificationEvidence{Type: EvidenciaConfianzaForzada, Summary: string(ConfianzaClaveRSACorta)}
	otraForzada := VerificationEvidence{Type: EvidenciaConfianzaForzada, Summary: string(ConfianzaCADesconocida)}
	casos := []struct {
		nombre    string
		detalles  []string
		evidencia []VerificationEvidence
		quiere    bool
	}{
		{"clave de 1024 sin excepción", []string{"subject=X", "rsa_bits=1024"}, nil, true},
		{"clave de 2047 sin excepción", []string{"rsa_bits=2047"}, nil, true},
		{"clave de 1024 con otra excepción", []string{"rsa_bits=1024"}, []VerificationEvidence{otraForzada}, true},
		{"clave de 1024 con la excepción aceptada", []string{"rsa_bits=1024"}, []VerificationEvidence{forzada}, false},
		{"clave de 2048", []string{"rsa_bits=2048"}, nil, false},
		{"sin detalle de bits", []string{"not_after=2020-01-01T00:00:00Z"}, nil, false},
		{"detalle mal formado", []string{"rsa_bits=mil"}, nil, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := VerificationResult{
				Certificate: VerificationAspect{Status: VerificationStatusInvalid, Details: c.detalles},
				Evidence:    c.evidencia,
			}
			if got := r.ClaveRSACortaSinExcepcion(); got != c.quiere {
				t.Fatalf("ClaveRSACortaSinExcepcion() = %v; quiere %v", got, c.quiere)
			}
		})
	}
}
