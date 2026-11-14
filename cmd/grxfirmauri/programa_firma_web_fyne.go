// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui && !windows

package main

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/ports"
)

func etiquetaAjustada(texto string, negrita bool) *widget.Label {
	label := widget.NewLabelWithStyle(texto, fyne.TextAlignLeading, fyne.TextStyle{Bold: negrita})
	label.Wrapping = fyne.TextWrapWord
	return label
}

// mostrarEleccionProgramaFirmaWeb pregunta con qué programa firmar. Cerrar la
// ventana equivale a GrxFirma sin recordar, que es lo que pasaba antes.
func mostrarEleccionProgramaFirmaWeb(_ context.Context, puedeRecordar bool) (eleccionProgramaFirmaWeb, error) {
	if err := checkPlatformGraphicsReadiness(); err != nil {
		return eleccionProgramaFirmaWeb{}, err
	}
	a := newLegacyFyneApp()
	w := a.NewWindow(tl("afirma.elegir.titulo"))
	eleccion := eleccionProgramaFirmaWeb{programa: ports.ProgramaAfirmaGrxFirma}
	recordar := widget.NewCheck(tl("afirma.elegir.recordar"), nil)
	elegir := func(programa string) func() {
		return func() {
			eleccion = eleccionProgramaFirmaWeb{programa: programa, recordar: puedeRecordar && recordar.Checked}
			w.Close()
		}
	}
	grx := widget.NewButton(tl("afirma.elegir.grxfirma"), elegir(ports.ProgramaAfirmaGrxFirma))
	grx.Importance = widget.HighImportance
	auto := widget.NewButton(tl("afirma.elegir.autofirma"), elegir(ports.ProgramaAfirmaAutoFirma))

	contenido := container.NewVBox(
		etiquetaAjustada(tl("afirma.elegir.pregunta"), true),
		etiquetaAjustada(tl("afirma.elegir.detalle"), false),
	)
	if puedeRecordar {
		contenido.Add(recordar)
		contenido.Add(etiquetaAjustada(tl("afirma.elegir.recordar_ayuda"), false))
	}
	botones := container.NewHBox(grx, auto)
	w.SetContent(container.NewPadded(container.NewBorder(nil, botones, nil, nil, contenido)))
	w.Resize(fyne.NewSize(560, 300))
	w.CenterOnScreen()
	w.Canvas().Focus(grx)
	w.ShowAndRun()
	return eleccion, nil
}

// mostrarFalloReenvio avisa de que AutoFirma no se abrió y ofrece firmar con
// GrxFirma. Cerrar la ventana es cancelar.
func mostrarFalloReenvio(context.Context) (bool, error) {
	if err := checkPlatformGraphicsReadiness(); err != nil {
		return false, err
	}
	a := newLegacyFyneApp()
	w := a.NewWindow(tl("afirma.elegir.titulo"))
	firmarAqui := false
	grx := widget.NewButton(tl("afirma.elegir.grxfirma"), func() {
		firmarAqui = true
		w.Close()
	})
	grx.Importance = widget.HighImportance
	cancelar := widget.NewButton(tl("afirma.elegir.cancelar"), w.Close)
	contenido := container.NewVBox(
		etiquetaAjustada(tl("afirma.reenvio.fallo"), true),
		etiquetaAjustada(tl("afirma.reenvio.fallo_detalle"), false),
	)
	w.SetContent(container.NewPadded(container.NewBorder(nil, container.NewHBox(grx, cancelar), nil, nil, contenido)))
	w.Resize(fyne.NewSize(560, 240))
	w.CenterOnScreen()
	w.Canvas().Focus(grx)
	w.ShowAndRun()
	return firmarAqui, nil
}
