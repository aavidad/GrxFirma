// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
	"errors"
)

func credentialLoadingAvailable() bool { return false }

func mostrarAvisoCredencialTemporal(context.Context, string, string) {}

func solicitarCredencialTemporal(ctx context.Context) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return nil, nil, errors.New("La carga de P12/PFX requiere la interfaz local de GrxFirma.")
}
