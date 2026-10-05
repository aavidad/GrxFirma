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
	"fyne.io/fyne/v2/storage"
	fynetest "fyne.io/fyne/v2/test"

	"grxfirma/internal/ports"
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

func TestFyneExtensionFilter(t *testing.T) {
	if fyneExtensionFilter(ports.DocumentFilter{}) != nil {
		t.Fatal("sin extensiones no debe haber filtro")
	}
	filtro := fyneExtensionFilter(ports.DocumentFilter{Extensions: []string{"pdf", ".XML", " "}})
	if filtro == nil {
		t.Fatal("falta el filtro")
	}
	if !filtro.Matches(storage.NewFileURI("/tmp/a.pdf")) || !filtro.Matches(storage.NewFileURI("/tmp/b.xml")) {
		t.Fatal("el filtro debe admitir .pdf y .xml")
	}
	if filtro.Matches(storage.NewFileURI("/tmp/c.txt")) {
		t.Fatal("el filtro no debe admitir .txt")
	}
}
