// SPDX-License-Identifier: EUPL-1.2
package ipc

import (
	"path/filepath"
	"testing"
)

func TestConfigDirSeguroNoUsaTmpNiRutaRelativa(t *testing.T) {
	t.Setenv("HOME", "")
	if got := (&Manejador{}).configDirSeguro(); got != "" {
		t.Fatalf("sin HOME abrió %q", got)
	}
	if got := (&Manejador{ConfigDir: "relative"}).configDirSeguro(); got != "" {
		t.Fatalf("aceptó ruta relativa %q", got)
	}
	absolute := t.TempDir()
	if got := (&Manejador{ConfigDir: absolute}).configDirSeguro(); got != filepath.Clean(absolute) {
		t.Fatalf("ruta absoluta: %q", got)
	}
}
