// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package appdirs

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigComprobadaRechazaPerfilOAppDataVacioORelativo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	for _, malo := range []string{"", " ", ".", "relativo"} {
		if dir, err := ConfigComprobada(malo); !errors.Is(err, ErrConfigNoDisponible) {
			t.Fatalf("home=%q produjo %q, %v", malo, dir, err)
		}
	}
	if dir, err := ConfigComprobada(home); err != nil || dir != Config(home) {
		t.Fatalf("perfil válido: %q, %v", dir, err)
	}
	for _, malo := range []string{"", ".", "relativo"} {
		t.Setenv("APPDATA", malo)
		dir, err := ConfigComprobada(home)
		if runtime.GOOS == "windows" {
			if !errors.Is(err, ErrConfigNoDisponible) {
				t.Fatalf("APPDATA=%q produjo %q, %v", malo, dir, err)
			}
		} else if err != nil {
			t.Fatalf("APPDATA no decide la configuración fuera de Windows: %v", err)
		}
	}
}

func TestConfigUsuarioRechazaAppDataRelativoEnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("solo Windows decide la configuración por %APPDATA%")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	for _, malo := range []string{"", ".", "relativo"} {
		t.Setenv("APPDATA", malo)
		if dir, err := ConfigUsuario(); !errors.Is(err, ErrConfigNoDisponible) {
			t.Fatalf("APPDATA=%q produjo %q, %v", malo, dir, err)
		}
	}
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	if dir, err := ConfigUsuario(); err != nil || dir != Config(home) {
		t.Fatalf("APPDATA absoluto: %q, %v", dir, err)
	}
}
