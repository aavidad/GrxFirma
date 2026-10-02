// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui && (!windows || !amd64)

package main

import (
	"context"
	"errors"
)

func solicitarPasswordTemporalNativo(context.Context) ([]byte, error) {
	return nil, errors.New("el diálogo nativo de Windows no está disponible en esta plataforma")
}

func mostrarAvisoCredencialTemporalNativo(context.Context, string, string) {}
