// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"grxfirma/internal/testsupport/perfil"
)

// setTestUserHome aísla el perfil completo (HOME, USERPROFILE, APPDATA,
// LOCALAPPDATA y XDG_*) y el temporal dentro de home, para que ninguna
// prueba lea ni escriba los ajustes reales del usuario en ningún sistema.
func setTestUserHome(t *testing.T, home string) {
	t.Helper()

	absHome := perfil.Aislar(t, home)
	tempDir := filepath.Join(absHome, "tmp")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(test temp): %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(absHome, "runtime"))
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
}

func sameTestPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func testProtocolConfigDir(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "GrxFirma")
	}
	return filepath.Join(home, ".config", "grxfirma")
}
