// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !darwin && !windows

package main

import (
	"context"
	"errors"
)

func seleccionarIndiceCertificadoSistema(
	context.Context,
	string,
	string,
	string,
	string,
	[]string,
) (int, error) {
	return 0, errors.New("la plataforma no dispone de selector local de certificados")
}
