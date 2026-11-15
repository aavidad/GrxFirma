//go:build darwin

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/testsupport/perfil"
)

func TestDarwinMayusculasNoPermitenLeerConfiguracion(t *testing.T) {
	home := perfil.Nuevo(t)
	root := filepath.Join(home, ".config", "grxfirma")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".CONFIG")); os.IsNotExist(err) {
		t.Skip("sistema de ficheros distingue mayúsculas")
	}
	for _, alias := range []string{filepath.Join(home, ".CONFIG", "grxfirma", "settings.json"), filepath.Join(home, ".config", "GRXFIRMA", "nuevo")} {
		if _, err := Check(alias); !errors.Is(err, ErrProtected) {
			t.Fatalf("alias admitido: %s %v", alias, err)
		}
	}
}
