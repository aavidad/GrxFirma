//go:build !fyne_gui

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Sin URI el binario gráfico inicia el servicio residente; este contrato de
// error corresponde únicamente a la variante sin interfaz gráfica.
func TestRunSinURI(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	code := run(context.Background(), nil, &stderr)
	if code == 0 {
		t.Fatal("run sin URI debe fallar")
	}
	output := stderr.String()
	if !strings.Contains(output, "afirma://") || !strings.Contains(strings.ToUpper(output), "URI") {
		t.Fatalf("mensaje de error inesperado: %q", output)
	}
}
