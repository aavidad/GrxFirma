// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package main

import (
	"context"
	"errors"
	"strconv"

	"grxfirma/internal/adapters/inbound/common/aprobacion"
	"grxfirma/internal/adapters/outbound/desktop/escritorio"
)

// dialogosSelectorIdentidadLinux devuelve los selectores candidatos, en orden,
// con los textos ya preparados para cada herramienta. zenity y qarma no
// admiten --no-markup en --list y pintan --text como marcado Pango: el texto
// se escapa (y las barras se duplican) con el componente común de aprobación.
// kdialog decide solo si el texto de --menu es HTML de Qt: se fuerza y se
// escapa igual que en el diálogo de firma (C-02). Las celdas de la lista son
// texto plano en las dos herramientas.
func dialogosSelectorIdentidadLinux(
	titulo string,
	prompt string,
	usar string,
	cancelar string,
	opciones []string,
) []dialogoLinux {
	textoZenity := aprobacion.TextoZenityMarcado(prompt)
	dialogos := make([]dialogoLinux, 0, 3)
	for _, programa := range []string{"zenity", "qarma"} {
		argumentos := []string{"--list", "--title", titulo, "--text", textoZenity,
			"--width", "1000", "--height", "520", "--hide-header",
			"--ok-label", usar, "--cancel-label", cancelar, "--column", "Índice",
			"--column", "Certificado", "--hide-column=1", "--print-column=1"}
		for indice, opcion := range opciones {
			argumentos = append(argumentos, strconv.Itoa(indice), opcion)
		}
		dialogos = append(dialogos, dialogoLinux{bin: programa, args: argumentos})
	}
	argumentos := []string{"--menu", aprobacion.TextoKDialog(prompt), "--title", titulo}
	for indice, opcion := range opciones {
		argumentos = append(argumentos, strconv.Itoa(indice), opcion)
	}
	return append(dialogos, dialogoLinux{bin: "kdialog", args: argumentos})
}

func seleccionarIndiceCertificadoSistema(
	ctx context.Context,
	titulo string,
	prompt string,
	usar string,
	cancelar string,
	opciones []string,
) (int, error) {
	if err := escritorio.Comprobar("selector_identidad_nativehost"); err != nil {
		return 0, err
	}
	for _, dialogo := range dialogosSelectorIdentidadLinux(titulo, prompt, usar, cancelar, opciones) {
		salida, err := ejecutarSelectorSistema(ctx, dialogo.bin, dialogo.args...)
		if err != nil {
			if errors.Is(err, errSeleccionIdentidadCancelada) {
				return 0, err
			}
			if dialogo.bin == "kdialog" {
				return 0, errors.New("no hay selector gráfico disponible para identidad reforzada")
			}
			continue
		}
		return strconv.Atoi(salida)
	}
	return 0, errors.New("no hay selector gráfico disponible para identidad reforzada")
}
