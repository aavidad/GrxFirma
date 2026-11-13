// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"strings"
	"testing"
)

func TestRenderEstadoWebSocketActivo_MuestraEstadoYDireccion(t *testing.T) {
	texto := renderEstadoWebSocketActivo("127.0.0.1:8083")

	for _, esperado := range []string{
		"SERVIDOR WEBSOCKET ACTIVO",
		"**Estado:** ACTIVO",
		"`wss://127.0.0.1:8083`",
		"Detener servidor",
	} {
		if !strings.Contains(texto, esperado) {
			t.Fatalf("el estado visible no contiene %q:\n%s", esperado, texto)
		}
	}
}

func TestRenderEstadoWebSocketActivo_UsaDireccionPorDefecto(t *testing.T) {
	texto := renderEstadoWebSocketActivo(" ")
	if !strings.Contains(texto, "`wss://127.0.0.1:8080`") {
		t.Fatalf("se esperaba dirección por defecto en el estado visible:\n%s", texto)
	}
}
