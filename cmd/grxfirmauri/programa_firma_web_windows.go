// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import (
	"context"

	"grxfirma/internal/adapters/outbound/desktop/wintaskdialog"
	"grxfirma/internal/ports"
)

// En Windows los diálogos son TaskDialog nativos en el mismo proceso: no
// arrancan Fyne y la ventana de GrxFirma puede abrirse después.
const (
	botonFirmaWebGrxFirma  int32 = 1001
	botonFirmaWebAutoFirma int32 = 1002
)

func preguntarProgramaFirmaWebPlataforma(ctx context.Context, puedeRecordar bool) (eleccionProgramaFirmaWeb, error) {
	return mostrarEleccionProgramaFirmaWeb(ctx, puedeRecordar)
}

func preguntarFalloReenvioPlataforma(ctx context.Context) (bool, error) {
	return mostrarFalloReenvio(ctx)
}

// mostrarEleccionProgramaFirmaWeb pregunta con qué programa firmar. Cerrar el
// diálogo equivale a GrxFirma sin recordar, que es lo que pasaba antes.
func mostrarEleccionProgramaFirmaWeb(ctx context.Context, puedeRecordar bool) (eleccionProgramaFirmaWeb, error) {
	if !wintaskdialog.Available() {
		return eleccionProgramaFirmaWeb{}, errDialogoNoDisponible
	}
	spec := wintaskdialog.Spec{
		Title:       tl("afirma.elegir.titulo"),
		Instruction: tl("afirma.elegir.pregunta"),
		Content:     tl("afirma.elegir.detalle"),
		Buttons: []wintaskdialog.Choice{
			{ID: botonFirmaWebGrxFirma, Label: tl("afirma.elegir.grxfirma")},
			{ID: botonFirmaWebAutoFirma, Label: tl("afirma.elegir.autofirma")},
		},
		DefaultButton: botonFirmaWebGrxFirma,
	}
	if puedeRecordar {
		spec.Verification = tl("afirma.elegir.recordar")
		spec.Content += "\n\n" + tl("afirma.elegir.recordar_ayuda")
	}
	boton, _, recordar, err := wintaskdialog.ShowVerification(ctx, spec)
	if err != nil {
		return eleccionProgramaFirmaWeb{}, err
	}
	recordar = recordar && puedeRecordar
	switch boton {
	case botonFirmaWebAutoFirma:
		return eleccionProgramaFirmaWeb{programa: ports.ProgramaAfirmaAutoFirma, recordar: recordar}, nil
	case botonFirmaWebGrxFirma:
		return eleccionProgramaFirmaWeb{programa: ports.ProgramaAfirmaGrxFirma, recordar: recordar}, nil
	}
	return eleccionProgramaFirmaWeb{programa: ports.ProgramaAfirmaGrxFirma}, nil
}

// mostrarFalloReenvio avisa de que AutoFirma no se abrió y ofrece firmar con
// GrxFirma. Cerrar el diálogo es cancelar.
func mostrarFalloReenvio(ctx context.Context) (bool, error) {
	if !wintaskdialog.Available() {
		return false, errDialogoNoDisponible
	}
	boton, _, err := wintaskdialog.Show(ctx, wintaskdialog.Spec{
		Title:       tl("afirma.elegir.titulo"),
		Instruction: tl("afirma.reenvio.fallo"),
		Content:     tl("afirma.reenvio.fallo_detalle"),
		Buttons: []wintaskdialog.Choice{
			{ID: botonFirmaWebGrxFirma, Label: tl("afirma.elegir.grxfirma")},
			{ID: wintaskdialog.IDCancel, Label: tl("afirma.elegir.cancelar")},
		},
		DefaultButton: botonFirmaWebGrxFirma,
	})
	if err != nil {
		return false, err
	}
	return boton == botonFirmaWebGrxFirma, nil
}
