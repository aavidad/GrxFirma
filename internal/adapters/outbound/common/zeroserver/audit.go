// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package zeroserver verifica el modelo Zero Server: WebSocket y REST están
// deshabilitados por defecto; afirma://, Native Messaging y CLI siempre activos.
//
// Esta auditoría se ejecuta al arrancar la aplicación y registra advertencias
// si el operador ha habilitado servidores opt-in sin la advertencia de seguridad.
package zeroserver

import (
	"context"
	"fmt"
	"log/slog"

	"grxfirma/internal/adapters/outbound/common/config"
)

// AuditResult contiene el resultado de la auditoría Zero Server.
type AuditResult struct {
	// ServidoresActivos es la lista de servidores con puertos abiertos activos.
	ServidoresActivos []string
	// Advertencias son mensajes de aviso para el operador.
	Advertencias []string
	// CumpleZeroServer es true solo si no hay ningún servidor TCP/UDP activo por defecto.
	CumpleZeroServer bool
}

// Auditor comprueba que la configuración cumple el modelo Zero Server.
type Auditor struct {
	logger *slog.Logger
}

// New crea un Auditor con el logger dado.
func New(logger *slog.Logger) *Auditor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Auditor{logger: logger}
}

// Audit evalúa la configuración y retorna el resultado de la auditoría.
// Registra advertencias en el logger si hay servidores opt-in activos.
func (a *Auditor) Audit(_ context.Context, cfg config.Config) AuditResult {
	result := AuditResult{CumpleZeroServer: true}

	if cfg.WebsocketHabilitado {
		result.ServidoresActivos = append(result.ServidoresActivos, "websocket")
		result.CumpleZeroServer = false
		msg := "WebSocket habilitado (opt-in): se abrirá un puerto TCP local. " +
			"Asegúrate de haber mostrado el diálogo de advertencia de seguridad al usuario."
		result.Advertencias = append(result.Advertencias, msg)
		a.logger.Warn("zeroserver: "+msg,
			slog.String("servidor", "websocket"),
			slog.String("accion_requerida", "mostrar_dialogo_seguridad"),
		)
	}

	if cfg.RestHabilitado {
		result.ServidoresActivos = append(result.ServidoresActivos, "rest")
		result.CumpleZeroServer = false
		msg := "REST API habilitada (opt-in): se abrirá un puerto TCP local. " +
			"Solo recomendado para entornos empresariales con TLS configurado."
		result.Advertencias = append(result.Advertencias, msg)
		a.logger.Warn("zeroserver: "+msg,
			slog.String("servidor", "rest"),
			slog.String("accion_requerida", "verificar_tls"),
		)
	}

	if result.CumpleZeroServer {
		a.logger.Info("zeroserver: configuración cumple modelo Zero Server (sin puertos abiertos por defecto)")
	}

	return result
}

// ResumenTexto retorna un resumen legible del resultado.
func (r AuditResult) ResumenTexto() string {
	if r.CumpleZeroServer {
		return "Zero Server: OK (sin puertos TCP/UDP abiertos)"
	}
	return fmt.Sprintf("Zero Server: ADVERTENCIA — servidores activos: %v", r.ServidoresActivos)
}
