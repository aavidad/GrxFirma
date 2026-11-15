// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package certpicker_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	fynetest "fyne.io/fyne/v2/test"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/certpicker"
)

func contieneTexto(object fyne.CanvasObject, texto string) bool {
	if t, ok := object.(*canvas.Text); ok && t.Text == texto {
		return true
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
		if contieneTexto(child, texto) {
			return true
		}
	}
	return false
}

// A-11: con acciones de credenciales (afirma://) la selección es siempre
// explícita y no se guarda, así que el selector no ofrece "Recordar".
func TestFyneSelector_SinRecordarConAccionesDeCredenciales(t *testing.T) {
	// El driver de prueba de Fyne hace de escritorio.
	t.Cleanup(escritorio.SustituirDetector(func() bool { return true }))
	t.Setenv("LANG", "es_ES.UTF-8")
	certpicker.SetConfigDir(t.TempDir())
	for _, conAcciones := range []bool{false, true} {
		a := fynetest.NewApp()
		w := &credentialTestWindow{Window: a.NewWindow("Test"), ready: make(chan struct{})}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		sel := certpicker.New().WithApp(a).WithWindow(w)
		if conAcciones {
			sel = sel.WithCredentialActions()
		}
		done := make(chan struct{})
		go func() {
			_, _ = sel.Select(ctx, []domain.CertificateRef{makeCert("test", "CN=Test", "CN=QA")})
			close(done)
		}()
		select {
		case <-w.ready:
		case <-ctx.Done():
			t.Fatal("no se creó UI")
		}
		hay := contieneTexto(w.Content(), "Recordar esta selección")
		cancel()
		<-done
		w.Close()
		a.Quit()
		if conAcciones && hay {
			t.Fatal("con acciones de credenciales no debe ofrecerse Recordar")
		}
		if !conAcciones && !hay {
			t.Fatal("control: sin acciones de credenciales el selector sí ofrece Recordar")
		}
	}
}
