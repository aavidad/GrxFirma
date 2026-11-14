// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import "testing"

// FuzzCambiosTrasFirmaPAdES comprueba que el análisis de revisiones
// posteriores nunca entra en pánico ante un PDF arbitrario y que, si no puede
// concluir, nunca deja la firma por buena.
func FuzzCambiosTrasFirmaPAdES(f *testing.F) {
	f.Add([]byte("%PDF-1.4\n1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\nxref\n0 2\n0000000000 65535 f\r\n0000000009 00000 n\r\ntrailer<</Size 2/Root 1 0 R>>\nstartxref\n52\n%%EOF\n"), 40)
	f.Add([]byte("3 0 obj\n<</Type/Page/Contents 4 0 R>>\nendobj\n3 0 obj\n<</Type/Page/Contents 5 0 R/Annots[6 0 R]>>\nendobj"), 38)
	f.Add([]byte("7 0 obj\n<</Type/ObjStm/N 1/First 4/Filter/FlateDecode/Length 3>>\nstream\nabc\nendstream\nendobj"), 0)
	f.Fuzz(func(t *testing.T, data []byte, corte int) {
		estado, _ := inspeccionarRevisionesPAdES(data).cambiosTrasFirma([4]int{0, corte, corte + 2, 0})
		switch estado {
		case cambiosPDFNinguno, cambiosPDFPermitidos, cambiosPDFNoComprobados, cambiosPDFNoPermitidos:
		default:
			t.Fatalf("estado desconocido %q", estado)
		}
	})
}
