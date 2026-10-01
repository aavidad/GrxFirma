// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package zeroserver_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/zeroserver"
)

func TestAudit_DefaultConfig_CumpleZeroServer(t *testing.T) {
	t.Parallel()
	a := zeroserver.New(nil)
	cfg := config.Default()
	res := a.Audit(context.Background(), cfg)
	if !res.CumpleZeroServer {
		t.Errorf("la configuración por defecto debe cumplir Zero Server: %v", res.Advertencias)
	}
	if len(res.ServidoresActivos) != 0 {
		t.Errorf("no debe haber servidores activos por defecto: %v", res.ServidoresActivos)
	}
}

func TestAudit_WebSocketHabilitado_NoZeroServer(t *testing.T) {
	t.Parallel()
	a := zeroserver.New(nil)
	cfg := config.Default()
	cfg.WebsocketHabilitado = true
	res := a.Audit(context.Background(), cfg)
	if res.CumpleZeroServer {
		t.Error("WebSocket habilitado no debe cumplir Zero Server")
	}
	if len(res.ServidoresActivos) != 1 || res.ServidoresActivos[0] != "websocket" {
		t.Errorf("esperado [websocket], obtenido %v", res.ServidoresActivos)
	}
	if len(res.Advertencias) == 0 {
		t.Error("debe haber advertencias con WebSocket activo")
	}
}

func TestAudit_RESTHabilitado_NoZeroServer(t *testing.T) {
	t.Parallel()
	a := zeroserver.New(nil)
	cfg := config.Default()
	cfg.RestHabilitado = true
	res := a.Audit(context.Background(), cfg)
	if res.CumpleZeroServer {
		t.Error("REST habilitado no debe cumplir Zero Server")
	}
	if len(res.ServidoresActivos) != 1 || res.ServidoresActivos[0] != "rest" {
		t.Errorf("esperado [rest], obtenido %v", res.ServidoresActivos)
	}
}

func TestAudit_AmbosHabilitados_DosAdvertencias(t *testing.T) {
	t.Parallel()
	a := zeroserver.New(nil)
	cfg := config.Default()
	cfg.WebsocketHabilitado = true
	cfg.RestHabilitado = true
	res := a.Audit(context.Background(), cfg)
	if res.CumpleZeroServer {
		t.Error("ambos habilitados no debe cumplir Zero Server")
	}
	if len(res.ServidoresActivos) != 2 {
		t.Errorf("esperados 2 servidores activos, obtenidos %d", len(res.ServidoresActivos))
	}
	if len(res.Advertencias) != 2 {
		t.Errorf("esperadas 2 advertencias, obtenidas %d", len(res.Advertencias))
	}
}

func TestAuditResult_ResumenTexto(t *testing.T) {
	t.Parallel()
	ok := zeroserver.AuditResult{CumpleZeroServer: true}
	if ok.ResumenTexto() == "" {
		t.Error("ResumenTexto() no debe ser vacío")
	}
	nok := zeroserver.AuditResult{CumpleZeroServer: false, ServidoresActivos: []string{"websocket"}}
	if nok.ResumenTexto() == ok.ResumenTexto() {
		t.Error("ResumenTexto() debe diferenciar OK de ADVERTENCIA")
	}
}
