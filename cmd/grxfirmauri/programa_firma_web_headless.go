// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui && !windows

package main

import "context"

func presentProtocolNoticePlatform(context.Context, string) {}

// Sin interfaz gráfica no se pregunta: firma GrxFirma, como antes.
func mostrarEleccionProgramaFirmaWeb(context.Context, bool) (eleccionProgramaFirmaWeb, error) {
	return eleccionProgramaFirmaWeb{}, errDialogoNoDisponible
}

func mostrarFalloReenvio(context.Context) (bool, error) {
	return false, errDialogoNoDisponible
}
