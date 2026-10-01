// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"grxfirma/presentation/desktop/certpicker"
)

type credentialPromptTestApp struct {
	fyne.App
	ready chan fyne.Window
}

func (a credentialPromptTestApp) NewWindow(title string) fyne.Window {
	return &credentialPromptTestWindow{Window: a.App.NewWindow(title), ready: a.ready}
}

type credentialPromptTestWindow struct {
	fyne.Window
	ready          chan fyne.Window
	closeIntercept func()
}

func (w *credentialPromptTestWindow) Show() {
	w.Window.Show()
	w.ready <- w
}

func (w *credentialPromptTestWindow) SetCloseIntercept(callback func()) {
	w.closeIntercept = callback
	w.Window.SetCloseIntercept(callback)
}

func findCredentialPasswordEntry(object fyne.CanvasObject) *widget.Entry {
	if entry, ok := object.(*widget.Entry); ok {
		return entry
	}
	if node, ok := object.(*fyne.Container); ok {
		for _, child := range node.Objects {
			if entry := findCredentialPasswordEntry(child); entry != nil {
				return entry
			}
		}
	}
	return nil
}

func TestTemporaryCredentialPasswordPromptClearsOnAcceptCancelAndContext(t *testing.T) {
	for _, action := range []string{"accept", "cancel", "context"} {
		t.Run(action, func(t *testing.T) {
			base := fynetest.NewApp()
			defer base.Quit()
			a := credentialPromptTestApp{App: base, ready: make(chan fyne.Window, 1)}
			setProtocolUIContext(a, nil)
			defer clearProtocolUIContext()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type response struct {
				password []byte
				err      error
			}
			result := make(chan response, 1)
			go func() { value, err := solicitarPasswordTemporalFyne(ctx); result <- response{value, err} }()
			var w fyne.Window
			select {
			case w = <-a.ready:
			case <-time.After(3 * time.Second):
				t.Fatal("prompt no creado")
			}
			entry := findCredentialPasswordEntry(w.Content())
			if entry == nil || !entry.Password {
				t.Fatal("contraseña no oculta")
			}
			entry.SetText("synthetic-secret")
			switch action {
			case "accept":
				entry.OnSubmitted(entry.Text)
			case "cancel":
				w.(*credentialPromptTestWindow).closeIntercept()
			case "context":
				cancel()
			}
			select {
			case got := <-result:
				defer clear(got.password)
				if entry.Text != "" {
					t.Fatal("campo conservó contraseña")
				}
				if action == "accept" {
					if got.err != nil || string(got.password) != "synthetic-secret" {
						t.Fatalf("respuesta: %v", got.err)
					}
				} else {
					if len(got.password) != 0 {
						t.Fatal("cancelación devolvió contraseña")
					}
					if !errors.Is(got.err, certpicker.ErrSeleccionCancelada) && !errors.Is(got.err, context.Canceled) {
						t.Fatalf("cancelación: %v", got.err)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("prompt no terminó")
			}
		})
	}
}
