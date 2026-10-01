// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func presentStartupTLSTrustFailurePlatform(message string) {
	if checkPlatformGraphicsReadiness() != nil {
		return
	}
	app := newLegacyFyneApp()
	window := app.NewWindow(tl("GrxFirma"))
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	closeButton := widget.NewButton(tl("Cerrar"), window.Close)
	window.SetContent(container.NewBorder(nil, closeButton, nil, nil, container.NewPadded(label)))
	window.Resize(fyne.NewSize(600, 250))
	window.CenterOnScreen()
	window.ShowAndRun()
}
