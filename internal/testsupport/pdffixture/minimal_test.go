// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pdffixture

import (
	"bytes"
	"testing"

	"github.com/digitorus/pdf"
)

func TestMinimalIsReadablePDF(t *testing.T) {
	raw := Minimal()
	if !bytes.Contains(raw, []byte(Marker)) {
		t.Fatal("minimal fixture does not contain its preservation marker")
	}
	if _, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatalf("minimal fixture is not a readable PDF: %v", err)
	}
}
