// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

import (
	"context"
	"errors"
)

func solicitarAprobacionWindows(context.Context, string, string) (bool, error) {
	return false, errors.New("el diálogo nativo de Windows solo está disponible en Windows")
}
