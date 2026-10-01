// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
)

type recordingLegacyLaunchApp struct {
	fyne.App
	events    *[]string
	duringRun func()
}

func (a recordingLegacyLaunchApp) Run() {
	*a.events = append(*a.events, "run")
	if a.duringRun != nil {
		a.duringRun()
	}
}

type recordingLegacyLaunchWindow struct {
	fyne.Window
	events *[]string
}

func (w recordingLegacyLaunchWindow) SetMaster() {
	*w.events = append(*w.events, "master")
	w.Window.SetMaster()
}

func (w recordingLegacyLaunchWindow) Show() {
	*w.events = append(*w.events, "show")
	w.Window.Show()
}

func (w recordingLegacyLaunchWindow) RequestFocus() {
	*w.events = append(*w.events, "focus")
	w.Window.RequestFocus()
}

func TestShowAndRunLegacyLaunchCreatesWindowBeforeTray(t *testing.T) {
	base := fynetest.NewApp()
	t.Cleanup(base.Quit)

	events := make([]string, 0, 5)
	var queued func()
	originalSchedule := scheduleLegacyUI
	scheduleLegacyUI = func(fn func()) {
		events = append(events, "tray-queued")
		queued = fn
	}
	t.Cleanup(func() {
		scheduleLegacyUI = originalSchedule
	})
	a := recordingLegacyLaunchApp{
		App:    base,
		events: &events,
		duringRun: func() {
			if queued != nil {
				queued()
			}
		},
	}
	w := recordingLegacyLaunchWindow{
		Window: base.NewWindow("GrxFirma — Firma web"),
		events: &events,
	}

	showAndRunLegacyLaunch(a, w, func() {
		events = append(events, "tray")
	})

	want := []string{"master", "show", "tray-queued", "run", "tray"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("orden de lifecycle = %v, want %v", events, want)
	}
	for _, event := range events {
		if event == "focus" {
			t.Fatal("la ventana de espera no debe robar el foco al navegador")
		}
	}
}

func TestConfigureLegacyLaunchDoesNotStartTrayByDefault(t *testing.T) {
	a := fynetest.NewApp()
	t.Cleanup(a.Quit)
	t.Cleanup(clearProtocolUIContext)
	w := a.NewWindow("GrxFirma — Firma web")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	scheduled := false
	originalSchedule := scheduleLegacyUI
	scheduleLegacyUI = func(func()) {
		scheduled = true
	}
	t.Cleanup(func() {
		scheduleLegacyUI = originalSchedule
	})

	startTray := configureLegacyLaunchWindow(
		a,
		w,
		ctx,
		cancel,
		"websocket",
		"127.0.0.1:63117",
		"session-test",
		t.TempDir(),
		true,
		false,
	)
	startTray()

	if scheduled {
		t.Fatal("la bandeja no debe arrancar si el modo residente está desactivado")
	}
}

func TestLegacyLaunchHelpAñadeAyudaDeNavegadorSoloACanalesLocales(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"websocket", "service"} {
		got := legacyLaunchHelp(mode)
		if len(got) != 5 {
			t.Fatalf("legacyLaunchHelp(%q) tiene %d bloques, want 5", mode, len(got))
		}
		last := got[len(got)-1]
		for _, want := range []string{"Firefox", "loopback", "TLS"} {
			if !strings.Contains(last, want) {
				t.Fatalf("legacyLaunchHelp(%q) no contiene %q: %q", mode, want, last)
			}
		}
	}
	if got := legacyLaunchHelp("directo"); len(got) != 4 {
		t.Fatalf("legacyLaunchHelp(directo) tiene %d bloques, want 4", len(got))
	}
	if !strings.Contains(legacyBrowserLocalAccessHelpID, "cierra por completo") {
		t.Fatalf("la ayuda base no incluye el reinicio completo del navegador: %q", legacyBrowserLocalAccessHelpID)
	}
}

func TestLegacyBrowserLocalAccessHelpTieneFallbackIngles(t *testing.T) {
	t.Parallel()

	got := protocolFallbackText("en", legacyBrowserLocalAccessHelpID)
	for _, want := range []string{"Firefox", "loopback", "local TLS trust", "fully close and reopen"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fallback inglés no contiene %q: %q", want, got)
		}
	}
	if strings.Contains(got, "Si usas") || strings.Contains(got, "cierra por completo") {
		t.Fatalf("fallback inglés conserva texto español: %q", got)
	}
}
