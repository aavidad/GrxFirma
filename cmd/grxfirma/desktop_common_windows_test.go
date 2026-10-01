// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestQtBackendCandidatesWindows_IncluyeWrapperExe(t *testing.T) {
	baseDir := filepath.Join(`C:\Program Files`, "GrxFirma")
	want := filepath.Join(baseDir, "grxfirma-gui.exe")
	if got := qtBackendCandidates(baseDir); !slices.Contains(got, want) {
		t.Fatalf("candidatos Windows = %#v; falta %q", got, want)
	}
}
