// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEffectiveLegacySaveDefaultPath_UsaAntecesorExistente(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	got := effectiveLegacySaveDefaultPath(filepath.Join(home, "Descargas", "firma.pdf"))
	want := filepath.Join(home, "firma.pdf")
	if got != want {
		t.Fatalf("effectiveLegacySaveDefaultPath() = %q, want %q", got, want)
	}
}

func TestLegacySaveAppleScript_EscapaRutaYNombre(t *testing.T) {
	if got, want := osaQuote(`ruta\con"comillas`), `ruta\\con\"comillas`; got != want {
		t.Fatalf("osaQuote() = %q, want %q", got, want)
	}

	script := legacySaveAppleScript(filepath.Join(t.TempDir(), `firma"final.pdf`))
	for _, expected := range []string{`choose file name`, `firma\"final.pdf`} {
		if !strings.Contains(script, expected) {
			t.Fatalf("AppleScript no contiene %q: %q", expected, script)
		}
	}
}

func TestLegacySaveZenityArgs_SoloSeleccionaRuta(t *testing.T) {
	defaultPath := filepath.Join(t.TempDir(), "firma.pdf")
	args := legacySaveZenityArgs(defaultPath, "pdf,xml")
	joined := strings.Join(args, "\n")

	for _, expected := range []string{
		"--file-selection",
		"--save",
		"--confirm-overwrite",
		"--filename=" + defaultPath,
		"*.pdf *.xml",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("argumentos Zenity no contienen %q: %#v", expected, args)
		}
	}
}

func TestLegacySaveKDialogArgs_SoloSeleccionaRuta(t *testing.T) {
	defaultPath := filepath.Join(t.TempDir(), "firma.pdf")
	args := legacySaveKDialogArgs(defaultPath, "pdf")

	if len(args) != 3 || args[0] != "--getsavefilename" || args[1] != defaultPath || args[2] != "*.pdf" {
		t.Fatalf("argumentos KDialog inesperados: %#v", args)
	}
}

func TestSelectLegacySaveTargetPath_UsaQarmaTrasErrorZenity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prueba dirigida al fallback Linux")
	}
	binDir := t.TempDir()
	writeLegacyDialogTestCommand(t, binDir, "zenity", "#!/bin/sh\nexit 255\n")
	writeLegacyDialogTestCommand(t, binDir, "qarma", "#!/bin/sh\nprintf '%s\\n' \"$GRXFIRMA_TEST_DIALOG_PATH\"\n")
	t.Setenv("PATH", binDir)
	selected := filepath.Join(t.TempDir(), "firma.pdf")
	t.Setenv("GRXFIRMA_TEST_DIALOG_PATH", selected)

	got, err := selectLegacySaveTargetPath(context.Background(), selected, "pdf")
	if err != nil {
		t.Fatalf("selectLegacySaveTargetPath() error = %v", err)
	}
	if got != selected {
		t.Fatalf("selectLegacySaveTargetPath() = %q, want %q", got, selected)
	}
}
