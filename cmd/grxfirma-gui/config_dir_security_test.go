// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/testsupport/perfil"
)

// La carpeta personal sale de HOME en Linux y macOS y de USERPROFILE en
// Windows: vacía o relativa, la interfaz no arranca con una configuración
// cualquiera.
func TestConfigDirGUIRechazaHomeVacioORelativo(t *testing.T) {
	for _, home := range []string{"", ".", "relative"} {
		t.Run(home, func(t *testing.T) {
			perfil.Nuevo(t)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			if dir, err := configDirGUI(); err == nil {
				t.Fatalf("perfil %q produjo configuración %q", home, dir)
			}
		})
	}
}

// En Windows la configuración la decide %APPDATA%: vacío o relativo se
// rechaza, igual que HOME en Linux, en vez de suponer una carpeta.
func TestConfigDirGUIRechazaAppDataVacioORelativoEnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("%APPDATA% solo decide la configuración en Windows")
	}
	for _, appdata := range []string{"", ".", "relative"} {
		t.Run(appdata, func(t *testing.T) {
			perfil.Nuevo(t)
			t.Setenv("APPDATA", appdata)
			if dir, err := configDirGUI(); err == nil {
				t.Fatalf("APPDATA=%q produjo configuración %q", appdata, dir)
			}
		})
	}
	home := perfil.Nuevo(t)
	if dir, err := configDirGUI(); err != nil || dir != filepath.Join(home, "AppData", "Roaming", "GrxFirma") {
		t.Fatalf("APPDATA válido: %q, %v", dir, err)
	}
}
