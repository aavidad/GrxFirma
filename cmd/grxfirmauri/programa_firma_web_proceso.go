// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	"grxfirma/internal/ports"
)

// Fuera de Windows los diálogos se muestran en un proceso aparte del mismo
// ejecutable (ver flagElegirProgramaFirmaWeb).
var ejecutableDialogoFirmaWeb = os.Executable

func ejecutarDialogoFirmaWeb(ctx context.Context, args ...string) (string, error) {
	self, err := ejecutableDialogoFirmaWeb()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, self, args...) // #nosec G204 -- el propio ejecutable con un modo interno fijo; no lleva el enlace.
	var out bytes.Buffer
	cmd.Stdout = &limiteEscritura{w: &out, n: 64}
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func preguntarProgramaFirmaWebPlataforma(ctx context.Context, puedeRecordar bool) (eleccionProgramaFirmaWeb, error) {
	args := []string{flagElegirProgramaFirmaWeb}
	if !puedeRecordar {
		args = append(args, flagSinRecordar)
	}
	salida, err := ejecutarDialogoFirmaWeb(ctx, args...)
	if err != nil {
		return eleccionProgramaFirmaWeb{}, err
	}
	return analizarEleccionFirmaWeb(salida)
}

func preguntarFalloReenvioPlataforma(ctx context.Context) (bool, error) {
	salida, err := ejecutarDialogoFirmaWeb(ctx, flagFalloReenvioAutoFirma)
	if err != nil {
		return false, err
	}
	switch salida {
	case ports.ProgramaAfirmaGrxFirma:
		return true, nil
	case respuestaCancelar:
		return false, nil
	}
	return false, errDialogoNoDisponible
}

// limiteEscritura corta la salida del proceso del diálogo: solo se esperan
// unas pocas palabras.
type limiteEscritura struct {
	w io.Writer
	n int
}

func (l *limiteEscritura) Write(p []byte) (int, error) {
	total := len(p)
	if l.n <= 0 {
		return total, nil
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	l.n -= len(p)
	if _, err := l.w.Write(p); err != nil {
		return 0, err
	}
	return total, nil
}
