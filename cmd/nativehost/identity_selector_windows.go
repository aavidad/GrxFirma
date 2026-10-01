// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import (
	"context"
	"errors"

	"grxfirma/internal/adapters/outbound/desktop/wintaskdialog"
)

const (
	botonUsarCertificado  int32 = 1001
	baseOpcionCertificado int32 = 2000
	maxOpcionesSelector         = 64
)

// seleccionarIndiceCertificadoSistema muestra una lista nativa de
// certificados (TaskDialog con opciones). Sustituye al formulario WinForms
// que se generaba con PowerShell -ExecutionPolicy Bypass.
func seleccionarIndiceCertificadoSistema(
	ctx context.Context,
	titulo string,
	prompt string,
	usar string,
	cancelar string,
	opciones []string,
) (int, error) {
	if len(opciones) == 0 {
		return 0, errors.New("no hay certificados locales seleccionables")
	}
	if len(opciones) > maxOpcionesSelector {
		return 0, errors.New("demasiados certificados para el selector local")
	}
	radios := make([]wintaskdialog.Choice, len(opciones))
	for i, opcion := range opciones {
		radios[i] = wintaskdialog.Choice{ID: baseOpcionCertificado + int32(i), Label: opcion}
	}
	boton, radio, err := wintaskdialog.Show(ctx, wintaskdialog.Spec{
		Title:            titulo,
		Instruction:      titulo,
		Content:          prompt,
		Buttons:          []wintaskdialog.Choice{{ID: botonUsarCertificado, Label: usar}, {ID: wintaskdialog.IDCancel, Label: cancelar}},
		DefaultButton:    wintaskdialog.IDCancel,
		Radios:           radios,
		AcceptNeedsRadio: botonUsarCertificado,
	})
	if err != nil {
		return 0, err
	}
	if boton != botonUsarCertificado {
		return 0, errSeleccionIdentidadCancelada
	}
	indice := int(radio - baseOpcionCertificado)
	if indice < 0 || indice >= len(opciones) {
		return 0, errors.New("selección local de certificado inválida")
	}
	return indice, nil
}
