// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setTestUserHome(t *testing.T, home string) {
	t.Helper()

	absHome, err := filepath.Abs(home)
	if err != nil {
		t.Fatalf("filepath.Abs(home): %v", err)
	}
	tempDir := filepath.Join(absHome, "tmp")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(test temp): %v", err)
	}

	volume := filepath.VolumeName(absHome)
	homePath := strings.TrimPrefix(absHome, volume)
	t.Setenv("HOME", absHome)
	t.Setenv("USERPROFILE", absHome)
	t.Setenv("HOMEDRIVE", volume)
	t.Setenv("HOMEPATH", homePath)
	t.Setenv("APPDATA", filepath.Join(absHome, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(absHome, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(absHome, ".config"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(absHome, "runtime"))
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	resolved, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir(): %v", err)
	}
	if !sameTestPath(resolved, absHome) {
		t.Fatalf("os.UserHomeDir() = %q, want %q", resolved, absHome)
	}
}

func sameTestPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}
