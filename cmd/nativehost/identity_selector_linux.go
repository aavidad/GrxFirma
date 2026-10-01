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
)

func seleccionarIndiceCertificadoSistema(
	ctx context.Context,
	titulo string,
	prompt string,
	usar string,
	cancelar string,
	opciones []string,
) (int, error) {
	for _, programa := range []string{"zenity", "qarma"} {
		argumentos := []string{"--list", "--title", titulo, "--text", prompt,
			"--width", "1000", "--height", "520", "--hide-header",
			"--ok-label", usar, "--cancel-label", cancelar, "--column", "Índice",
			"--column", "Certificado", "--hide-column=1", "--print-column=1"}
		for indice, opcion := range opciones {
			argumentos = append(argumentos, strconv.Itoa(indice), opcion)
		}
		salida, err := ejecutarSelectorSistema(ctx, programa, argumentos...)
		if err != nil {
			if errors.Is(err, errSeleccionIdentidadCancelada) {
				return 0, err
			}
			continue
		}
		return strconv.Atoi(salida)
	}
	argumentos := []string{"--menu", prompt, "--title", titulo}
	for indice, opcion := range opciones {
		argumentos = append(argumentos, strconv.Itoa(indice), opcion)
	}
	salida, err := ejecutarSelectorSistema(ctx, "kdialog", argumentos...)
	if err != nil {
		if errors.Is(err, errSeleccionIdentidadCancelada) {
			return 0, err
		}
		return 0, errors.New("no hay selector gráfico disponible para identidad reforzada")
	}
	return strconv.Atoi(salida)
}
