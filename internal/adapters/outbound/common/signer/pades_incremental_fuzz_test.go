// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import "testing"

// FuzzAnalyzePDFIncrementalUpdate comprueba que el análisis de revisiones
// posteriores nunca entra en pánico ante un PDF arbitrario.
func FuzzAnalyzePDFIncrementalUpdate(f *testing.F) {
	f.Add([]byte("1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\ntrailer<</Root 1 0 R>>\n"), 40)
	f.Add([]byte("3 0 obj\n<</Type/Page/Contents 4 0 R>>\nendobj\n3 0 obj\n<</Type/Page/Contents 5 0 R/Annots[6 0 R]>>\nendobj"), 38)
	f.Add([]byte("7 0 obj\n<</Type/ObjStm/N 1/First 4/Filter/FlateDecode/Length 3>>\nstream\nabc\nendstream\nendobj"), 0)
	f.Fuzz(func(t *testing.T, data []byte, corte int) {
		if corte < 0 || corte > len(data) {
			return
		}
		_ = analyzePDFIncrementalUpdate(data, corte)
	})
}
