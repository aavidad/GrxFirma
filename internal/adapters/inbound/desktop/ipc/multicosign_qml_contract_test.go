// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQML_CofirmaMultipleSoloSeOfreceEnFormatosCompatibles(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "cmd", "gui-qml", "qml", "main.qml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se pudo leer main.qml: %v", err)
	}
	qml := string(raw)
	for _, snippet := range []string{
		`function supportsGuidedMultiCosignFormat()`,
		`format === "pades" || format === "odf" || format === "ooxml"`,
		`visible: signAction !== "countersign" && window.supportsGuidedMultiCosignFormat()`,
		`enabled: window.supportsGuidedMultiCosignFormat()`,
		`if (multiCosignEnabled && !supportsGuidedMultiCosignFormat())`,
	} {
		if !strings.Contains(qml, snippet) {
			t.Errorf("main.qml no conserva el contrato de formatos de cofirma múltiple: falta %q", snippet)
		}
	}
}
