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

// A-09: el aprobador de escritorio admite otro título para el guardado.
func TestAprobacionInteractivaConTitulo(t *testing.T) {
	var aprobador any = aprobacionInteractiva{titulo: "firma"}
	conTitulo, ok := aprobador.(aprobacionConTitulo)
	if !ok {
		t.Fatal("aprobacionInteractiva no admite otro título")
	}
	otro, ok := conTitulo.ConTitulo("guardado").(aprobacionInteractiva)
	if !ok || otro.titulo != "guardado" || aprobador.(aprobacionInteractiva).titulo != "firma" {
		t.Fatalf("ConTitulo = %#v", otro)
	}
}

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
