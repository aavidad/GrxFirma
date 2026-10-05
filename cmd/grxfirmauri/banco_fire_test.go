// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build firebanco && !fyne_gui

package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestBancoFIRe_ProcesaURIReal recibe la URL afirma:// que el autoscript.js
// oficial de FIRe generó en el banco local (scripts/fire-banco) y la procesa
// con el mismo manejador que usa grxfirmauri. Solo sustituye el diálogo de
// confirmación por un aprobador de prueba: el binario sin interfaz gráfica
// rechaza siempre las firmas web, y no se añade ninguna puerta trasera por
// variables de entorno al ejecutable.
//
// Se compila solo con la etiqueta firebanco y se omite sin FIRE_BANCO_URI, de
// modo que nunca entra en go test ./...
func TestBancoFIRe_ProcesaURIReal(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("FIRE_BANCO_URI"))
	if uri == "" {
		t.Skip("FIRE_BANCO_URI no definida: esta prueba la lanza scripts/fire-banco/probar-lote.sh")
	}
	if !strings.HasPrefix(uri, "afirma://") {
		t.Fatalf("FIRE_BANCO_URI no es una URL afirma://")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var stderr bytes.Buffer
	code := runE2EAislado(ctx, []string{uri}, &stderr)
	t.Logf("salida del manejador:\n%s", stderr.String())
	if code != 0 {
		t.Fatalf("el manejador afirma:// terminó con código %d", code)
	}
}
