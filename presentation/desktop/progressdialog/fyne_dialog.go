// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package progressdialog

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// FyneProgressDialog implementa ProgressProvider mostrando un diálogo modal Fyne.
type FyneProgressDialog struct {
	window fyne.Window
}

// NewFyneProgressDialog crea el proveedor de progreso con la ventana padre.
func NewFyneProgressDialog(w fyne.Window) *FyneProgressDialog {
	return &FyneProgressDialog{window: w}
}

type fyneReporter struct {
	label    *canvas.Text
	progress *widget.ProgressBar
	dlg      *dialog.CustomDialog
}

func (r *fyneReporter) SetMensaje(msg string) {
	fyne.Do(func() {
		r.label.Text = msg
		r.label.Refresh()
	})
}

func (r *fyneReporter) SetProgreso(fraccion float64) {
	if fraccion < 0 {
		return
	}
	fyne.Do(func() {
		r.progress.SetValue(fraccion)
	})
}

func (r *fyneReporter) Cerrar() {
	fyne.Do(func() {
		r.dlg.Hide()
	})
}

var _ Reporter = (*fyneReporter)(nil)

func (f *FyneProgressDialog) MostrarProgreso(ctx context.Context, titulo string) Reporter {
	lbl := canvas.NewText("Iniciando…", theme.Color(theme.ColorNameForeground))
	lbl.TextSize = 30
	lbl.TextStyle = fyne.TextStyle{Bold: true}
	bar := widget.NewProgressBar()
	content := container.NewPadded(container.NewVBox(lbl, bar))

	var d *dialog.CustomDialog
	fyne.DoAndWait(func() {
		d = dialog.NewCustomWithoutButtons(titulo, content, f.window)
		d.Resize(fyne.NewSize(1680, 720))
		d.Show()
	})

	r := &fyneReporter{label: lbl, progress: bar, dlg: d}
	go func() {
		<-ctx.Done()
		fyne.Do(func() {
			d.Hide()
		})
	}()

	return r
}

var _ ProgressProvider = (*FyneProgressDialog)(nil)
