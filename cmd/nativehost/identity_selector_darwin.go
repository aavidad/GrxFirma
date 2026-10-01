// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package main

import (
	"context"
	"errors"
)

func seleccionarIndiceCertificadoSistema(
	ctx context.Context,
	titulo string,
	prompt string,
	usar string,
	cancelar string,
	opciones []string,
) (int, error) {
	argumentos := []string{"-e", `on run argv
set titulo to item 1 of argv
set mensaje to item 2 of argv
set usar to item 3 of argv
set cancelar to item 4 of argv
set opciones to items 5 thru -1 of argv
set elegida to choose from list opciones with title titulo with prompt mensaje OK button name usar cancel button name cancelar
if elegida is false then error number -128
return item 1 of elegida
end run`, titulo, prompt, usar, cancelar}
	argumentos = append(argumentos, opciones...)
	salida, err := ejecutarSelectorSistema(ctx, "osascript", argumentos...)
	if err != nil {
		return 0, err
	}
	for indice, opcion := range opciones {
		if salida == opcion {
			return indice, nil
		}
	}
	return 0, errors.New("el selector devolvió un certificado desconocido")
}
