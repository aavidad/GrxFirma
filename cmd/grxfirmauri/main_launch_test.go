// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
)

func TestParseWebSocketLaunchURI(t *testing.T) {
	req, err := parseWebSocketLaunchURI("afirma://websocket?ports=63117,63118&idsession=abc123&v=4")
	if err != nil {
		t.Fatalf("error inesperado parseando websocket launch uri: %v", err)
	}
	if req == nil {
		t.Fatalf("request nil")
	}
	if req.Version != 4 {
		t.Fatalf("version inesperada: %d", req.Version)
	}
	if req.SessionID != "abc123" {
		t.Fatalf("idsession inesperada: %q", req.SessionID)
	}
	if len(req.Ports) != 2 || req.Ports[0] != 63117 || req.Ports[1] != 63118 {
		t.Fatalf("puertos inesperados: %v", req.Ports)
	}
}

func TestParseWebSocketLaunchURIAliasesAndDefaults(t *testing.T) {
	req, err := parseWebSocketLaunchURI("AFIRMA:///websocket?port=63119&idSession=sid01")
	if err != nil {
		t.Fatalf("error inesperado en aliases/defaults: %v", err)
	}
	if req.Version != 4 {
		t.Fatalf("sin v debe usar default 4, obtenido=%d", req.Version)
	}
	if req.SessionID != "sid01" {
		t.Fatalf("idSession no mapeada: %q", req.SessionID)
	}
	if len(req.Ports) != 1 || req.Ports[0] != 63119 {
		t.Fatalf("puerto inesperado: %v", req.Ports)
	}
}

func TestParseWebSocketLaunchURIActionInQueryOp(t *testing.T) {
	req, err := parseWebSocketLaunchURI("afirma://?op=websocket&portsList=63117&idSession=sid02")
	if err != nil {
		t.Fatalf("error inesperado en websocket por op query: %v", err)
	}
	if req.SessionID != "sid02" {
		t.Fatalf("idSession inesperada: %q", req.SessionID)
	}
	if len(req.Ports) != 1 || req.Ports[0] != 63117 {
		t.Fatalf("puertos inesperados: %v", req.Ports)
	}
}

func TestParseWebSocketLaunchURIUnsupportedVersion(t *testing.T) {
	if _, err := parseWebSocketLaunchURI("afirma://websocket?ports=63117&v=2"); err == nil {
		t.Fatalf("se esperaba error con version websocket no soportada")
	}
}

func TestParseWebSocketLaunchURIMissingPortsUsesDefault(t *testing.T) {
	req, err := parseWebSocketLaunchURI("afirma://websocket?idsession=abc&v=4")
	if err != nil {
		t.Fatalf("error inesperado sin puertos: %v", err)
	}
	if len(req.Ports) != 1 || req.Ports[0] != defaultLaunchedWebSocketPort {
		t.Fatalf("sin puertos debe usar default %d, obtenido=%v", defaultLaunchedWebSocketPort, req.Ports)
	}
}

func TestParseServiceLaunchURI(t *testing.T) {
	req, err := parseServiceLaunchURI("afirma://service?ports=63117,63118&idsession=svc123&v=3")
	if err != nil {
		t.Fatalf("error inesperado parseando service launch uri: %v", err)
	}
	if req == nil {
		t.Fatalf("request nil")
	}
	if req.Version != 3 {
		t.Fatalf("version inesperada: %d", req.Version)
	}
	if req.SessionID != "svc123" {
		t.Fatalf("idsession inesperada: %q", req.SessionID)
	}
	if len(req.Ports) != 2 || req.Ports[0] != 63117 || req.Ports[1] != 63118 {
		t.Fatalf("puertos inesperados: %v", req.Ports)
	}
}

func TestParseServiceLaunchURIUnsupportedVersion(t *testing.T) {
	if _, err := parseServiceLaunchURI("afirma://service?ports=63117&v=4"); err == nil {
		t.Fatalf("se esperaba error con version service no soportada")
	}
}

func TestParseServiceLaunchURIMissingPorts(t *testing.T) {
	if _, err := parseServiceLaunchURI("afirma://service?idsession=abc&v=3"); err == nil {
		t.Fatalf("se esperaba error cuando faltan puertos")
	}
}

func TestParseServiceLaunchURIMissingSession(t *testing.T) {
	if _, err := parseServiceLaunchURI("afirma://service?ports=63117&v=3"); err == nil {
		t.Fatal("se esperaba error cuando falta idsession")
	}
}

func TestShouldAutoCloseLegacyLaunch(t *testing.T) {
	tests := []struct {
		name   string
		op     afirmauri.TipoOperacion
		result string
		want   bool
	}{
		{name: "save ok", op: afirmauri.OperacionSave, result: "SAVE_OK", want: true},
		{name: "signandsave ok", op: afirmauri.OperacionSignSave, result: "OK", want: true},
		{name: "save cancel", op: afirmauri.OperacionSave, result: "CANCEL", want: false},
		{name: "save saf error", op: afirmauri.OperacionSave, result: "SAF_03:=Parametros incorrectos", want: false},
		{name: "sign no auto close", op: afirmauri.OperacionFirma, result: "OK", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAutoCloseLegacyLaunch(tc.op, tc.result); got != tc.want {
				t.Fatalf("shouldAutoCloseLegacyLaunch(%q, %q) = %t, want %t", tc.op, tc.result, got, tc.want)
			}
		})
	}
}
