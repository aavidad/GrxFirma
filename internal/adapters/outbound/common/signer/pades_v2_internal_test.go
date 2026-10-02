// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import "testing"

func TestParsePDFDictValue_RechazaClaveDuplicada(t *testing.T) {
	if _, ok := parsePDFDictValue("<< /Type /Catalog /Pages 2 0 R /Pages 9 0 R >>"); ok {
		t.Fatal("un catálogo ambiguo no puede clasificarse como cambio permitido")
	}
}
