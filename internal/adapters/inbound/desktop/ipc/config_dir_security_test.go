// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/testsupport/perfil"
)

func TestConfigDirSeguroNoUsaTmpNiRutaRelativa(t *testing.T) {
	perfil.Nuevo(t)
	// La carpeta personal es HOME en Linux y macOS y USERPROFILE en Windows.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if got := (&Manejador{}).configDirSeguro(); got != "" {
		t.Fatalf("sin carpeta personal abrió %q", got)
	}
	if got := (&Manejador{ConfigDir: "relative"}).configDirSeguro(); got != "" {
		t.Fatalf("aceptó ruta relativa %q", got)
	}
	absolute := t.TempDir()
	if got := (&Manejador{ConfigDir: absolute}).configDirSeguro(); got != filepath.Clean(absolute) {
		t.Fatalf("ruta absoluta: %q", got)
	}
}

// En Windows %APPDATA% decide la configuración: vacío o relativo se rechaza
// como HOME en Linux.
func TestConfigDirSeguroRechazaAppDataVacioORelativoEnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("%APPDATA% solo decide la configuración en Windows")
	}
	for _, appdata := range []string{"", ".", "relative"} {
		perfil.Nuevo(t)
		t.Setenv("APPDATA", appdata)
		if got := (&Manejador{}).configDirSeguro(); got != "" {
			t.Fatalf("APPDATA=%q abrió %q", appdata, got)
		}
	}
	home := perfil.Nuevo(t)
	if got := (&Manejador{}).configDirSeguro(); got != filepath.Join(home, "AppData", "Roaming", "GrxFirma") {
		t.Fatalf("APPDATA válido: %q", got)
	}
}
