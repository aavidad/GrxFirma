// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package certpicker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/certpicker"
)

type credentialTestWindow struct {
	fyne.Window
	ready chan struct{}
	once  sync.Once
}

func (w *credentialTestWindow) SetContent(content fyne.CanvasObject) {
	w.Window.SetContent(content)
	w.once.Do(func() { close(w.ready) })
}

func credentialButton(object fyne.CanvasObject, label string) *widget.Button {
	if button, ok := object.(*widget.Button); ok && button.Text == label {
		return button
	}
	var children []fyne.CanvasObject
	switch node := object.(type) {
	case *fyne.Container:
		children = node.Objects
	case *container.Split:
		children = []fyne.CanvasObject{node.Leading, node.Trailing}
	case *container.Scroll:
		children = []fyne.CanvasObject{node.Content}
	}
	for _, child := range children {
		if result := credentialButton(child, label); result != nil {
			return result
		}
	}
	return nil
}

func TestFyneCredentialActionsAvailableWithEmptyAndSingleCatalog(t *testing.T) {
	t.Setenv("LANG", "es_ES.UTF-8")
	certpicker.SetConfigDir(t.TempDir())
	for _, count := range []int{0, 1} {
		for _, action := range []struct {
			label    string
			expected error
		}{
			{"Usar un archivo P12/PFX…", certpicker.ErrCargarCertificado},
			{"Actualizar certificados", certpicker.ErrActualizarCertificados},
			{"Cancelar", certpicker.ErrSeleccionCancelada},
		} {
			t.Run(action.label+string(rune('0'+count)), func(t *testing.T) {
				a := fynetest.NewApp()
				defer a.Quit()
				w := &credentialTestWindow{Window: a.NewWindow("Test"), ready: make(chan struct{})}
				defer w.Close()
				var certs []domain.CertificateRef
				if count == 1 {
					certs = []domain.CertificateRef{makeCert("test", "CN=Test", "CN=QA")}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				result := make(chan error, 1)
				go func() {
					_, err := certpicker.New().WithApp(a).WithWindow(w).WithCredentialActions().Select(ctx, certs)
					result <- err
				}()
				select {
				case <-w.ready:
				case err := <-result:
					t.Fatalf("selector se cerró sin UI: %v", err)
				case <-ctx.Done():
					t.Fatal("no se creó UI")
				}
				button := credentialButton(w.Content(), action.label)
				if button == nil || button.Disabled() {
					t.Fatal("acción local ausente o deshabilitada")
				}
				fynetest.Tap(button)
				select {
				case err := <-result:
					if !errors.Is(err, action.expected) {
						t.Fatalf("resultado: %v", err)
					}
				case <-ctx.Done():
					t.Fatal("selector no terminó tras acción")
				}
			})
		}
	}
}
