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
