// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

func ejecutarSelectorSistema(
	ctx context.Context,
	programa string,
	argumentos ...string,
) (string, error) {
	ruta, err := exec.LookPath(programa)
	if err != nil {
		return "", err
	}
	// #nosec G204 -- programa procede de listas fijas y los textos se pasan como argv.
	comando := exec.CommandContext(ctx, ruta, argumentos...)
	salida, err := comando.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var salidaProceso *exec.ExitError
		if errors.As(err, &salidaProceso) && salidaProceso.ExitCode() == 1 {
			return "", errSeleccionIdentidadCancelada
		}
		return "", err
	}
	return strings.TrimSpace(string(salida)), nil
}
