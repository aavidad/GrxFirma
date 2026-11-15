// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package excepciones

import (
	"errors"
	"testing"
)

func TestConfigDirRechazaAppDataVacioORelativo(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	for _, malo := range []string{"", ".", "relativo"} {
		t.Setenv("APPDATA", malo)
		if dir, err := configDirForOS("windows"); !errors.Is(err, ErrConfigDirectory) || dir != "" {
			t.Fatalf("APPDATA=%q produjo %q, %v", malo, dir, err)
		}
	}
}
