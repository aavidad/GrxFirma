// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLegacyLoadPathsFromCommandOutput_ConservaPipeEnNombre(t *testing.T) {
	first := filepath.Join(t.TempDir(), "firma|final.pdf")
	second := filepath.Join(t.TempDir(), "otra.pdf")
	out := []byte(first + legacyLoadGTKSeparator + second + "\n")

	got, err := legacyLoadPathsFromCommandOutput(out, true, legacyLoadGTKSeparator)
	if err != nil {
		t.Fatalf("legacyLoadPathsFromCommandOutput() error = %v", err)
	}
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("rutas multiselección inesperadas: %#v", got)
	}
}

func TestLegacyLoadGTKArgs_UsaSeparadorNoAmbiguoParaPipe(t *testing.T) {
	args := legacyLoadGTKArgs("pdf", true)
	want := "--separator=" + legacyLoadGTKSeparator
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("legacyLoadGTKArgs() no contiene %q: %#v", want, args)
}

func TestDialogCancellation_DistingueCancelacionDeError(t *testing.T) {
	exitOne := commandOutputError(t, "exit 1")
	if !isLinuxDialogCancelErr(exitOne) {
		t.Fatal("exit 1 de un diálogo Linux debe representar cancelación")
	}
	if isAppleScriptCancelErr(exitOne) {
		t.Fatal("un exit 1 genérico de osascript no debe representar cancelación")
	}

	exit255 := commandOutputError(t, "exit 255")
	if isLinuxDialogCancelErr(exit255) {
		t.Fatal("exit 255 de un diálogo Linux debe conservarse como error")
	}

	appleCancel := commandOutputError(t, "printf 'execution error: User canceled. (-128)\\n' >&2; exit 1")
	if !isAppleScriptCancelErr(appleCancel) {
		t.Fatal("el error AppleScript -128 debe representar cancelación")
	}
}

func TestSelectLegacyLoadPaths_UsaQarmaTrasErrorZenity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prueba dirigida al fallback Linux")
	}
	binDir := t.TempDir()
	writeLegacyDialogTestCommand(t, binDir, "zenity", "#!/bin/sh\nexit 255\n")
	writeLegacyDialogTestCommand(t, binDir, "qarma", "#!/bin/sh\nprintf '%s\\n' \"$GRXFIRMA_TEST_DIALOG_PATH\"\n")
	t.Setenv("PATH", binDir)
	selected := filepath.Join(t.TempDir(), "firma.pdf")
	t.Setenv("GRXFIRMA_TEST_DIALOG_PATH", selected)

	got, err := selectLegacyLoadPaths(context.Background(), "", "pdf", false)
	if err != nil {
		t.Fatalf("selectLegacyLoadPaths() error = %v", err)
	}
	if len(got) != 1 || got[0] != selected {
		t.Fatalf("selectLegacyLoadPaths() = %#v, want %q", got, selected)
	}
}

func TestSelectLegacyLoadPaths_CancelacionZenityNoAbreFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prueba dirigida al fallback Linux")
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "qarma-invocado")
	writeLegacyDialogTestCommand(t, binDir, "zenity", "#!/bin/sh\nexit 1\n")
	writeLegacyDialogTestCommand(t, binDir, "qarma", "#!/bin/sh\n: > \"$GRXFIRMA_TEST_DIALOG_MARKER\"\n")
	t.Setenv("PATH", binDir)
	t.Setenv("GRXFIRMA_TEST_DIALOG_MARKER", marker)

	_, err := selectLegacyLoadPaths(context.Background(), "", "pdf", false)
	if !errors.Is(err, errLegacyLoadCanceled) {
		t.Fatalf("selectLegacyLoadPaths() error = %v, want cancelación", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Qarma se invocó después de una cancelación real: %v", statErr)
	}
}

func TestSelectLegacyLoadPaths_UsaKDialogTrasErroresGTK(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prueba dirigida al fallback Linux")
	}
	binDir := t.TempDir()
	writeLegacyDialogTestCommand(t, binDir, "zenity", "#!/bin/sh\nexit 255\n")
	writeLegacyDialogTestCommand(t, binDir, "qarma", "#!/bin/sh\nexit 255\n")
	writeLegacyDialogTestCommand(t, binDir, "kdialog", "#!/bin/sh\nprintf '%s\\n' \"$GRXFIRMA_TEST_DIALOG_PATH\"\n")
	t.Setenv("PATH", binDir)
	selected := filepath.Join(t.TempDir(), "firma.pdf")
	t.Setenv("GRXFIRMA_TEST_DIALOG_PATH", selected)

	got, err := selectLegacyLoadPaths(context.Background(), "", "pdf", false)
	if err != nil {
		t.Fatalf("selectLegacyLoadPaths() error = %v", err)
	}
	if len(got) != 1 || got[0] != selected {
		t.Fatalf("selectLegacyLoadPaths() = %#v, want %q", got, selected)
	}
}

func commandOutputError(t *testing.T, script string) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	_, err := cmd.Output()
	if err == nil {
		t.Fatalf("el comando de prueba no devolvió error: %q", script)
	}
	return err
}

func writeLegacyDialogTestCommand(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("no se pudo crear %s: %v", name, err)
	}
}
