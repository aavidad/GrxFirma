// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A-09: tras guardar desde la web queda la marca Zone.Identifier con zona 3.
func TestGuardadoWeb_MarcaZoneIdentifierWindows(t *testing.T) {
	destino := filepath.Join(carpetaGuardadoWeb(t), "doc.pdf")
	if resp := guardarDesdeWeb(t, destino, nil); resp != "SAVE_OK" {
		t.Fatalf("handleSave() = %q", resp)
	}
	raw, err := os.ReadFile(destino + ":Zone.Identifier")
	if err != nil {
		t.Skipf("el sistema de ficheros no admite flujos alternativos: %v", err)
	}
	if !strings.Contains(string(raw), "ZoneId=3") {
		t.Fatalf("Zone.Identifier = %q", raw)
	}
}
