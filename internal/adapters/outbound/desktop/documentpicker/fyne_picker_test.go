// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package documentpicker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
)

type signalingParentWindow struct {
	fyne.Window
	shown chan struct{}
	once  sync.Once
}

func (w *signalingParentWindow) Show() {
	w.Window.Show()
	w.once.Do(func() { close(w.shown) })
}

func TestFyneResolverReusesVisibleParentWindow(t *testing.T) {
	a := fynetest.NewApp()
	t.Cleanup(a.Quit)

	parent := a.NewWindow("GrxFirma — Firma web")
	parent.Show()
	parentSignal := &signalingParentWindow{
		Window: parent,
		shown:  make(chan struct{}),
	}
	initialWindows := len(a.Driver().AllWindows())

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := (fyneResolver{parent: func() fyne.Window {
			return parentSignal
		}}).ResolveDocument(ctx)
		result <- err
	}()

	select {
	case <-parentSignal.shown:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("el selector no hizo visible la ventana principal")
	}
	if got := len(a.Driver().AllWindows()); got != initialWindows {
		cancel()
		t.Fatalf("ResolveDocument creó %d ventanas adicionales; debe reutilizar la principal", got-initialWindows)
	}

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveDocument() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ResolveDocument no terminó al cancelar el contexto")
	}

	if got := len(a.Driver().AllWindows()); got != initialWindows {
		t.Fatalf("la cancelación cerró la ventana principal: ventanas=%d, want=%d", got, initialWindows)
	}
}
