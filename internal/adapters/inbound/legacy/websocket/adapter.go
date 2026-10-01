// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
)

const (
	// EchoRequestPrefix identifica el mensaje de salud usado por el flujo legacy.
	EchoRequestPrefix = "echo="

	// EchoResponseOK es la respuesta esperada por integraciones antiguas.
	EchoResponseOK = "OK"
)

// SignDocumentUseCase define el contrato minimo esperado por el flujo de firma directa.
type SignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

// RequestHandler ejecuta una solicitud legacy completa, incluyendo retrieve/upload.
type RequestHandler interface {
	HandleRequest(ctx context.Context, solicitud afirmauri.Solicitud) error
}

// LegacyMessageHandler permite reproducir la respuesta observable de V1
// en el borde websocket/service cuando no basta con devolver "OK".
type LegacyMessageHandler interface {
	HandleLegacy(ctx context.Context, raw string, solicitud afirmauri.Solicitud) (Resultado, bool, error)
}

// Resultado describe la salida traducida del mensaje heredado.
type Resultado struct {
	Tipo                string
	Texto               string
	Operacion           afirmauri.TipoOperacion
	Solicitud           *afirmauri.Solicitud
	FirmaBase64         string
	Formato             string
	CertificateID       string
	RequiereIntercambio bool
}

// Adaptador implementa la traduccion de mensajes heredados WebSocket.
type Adaptador struct {
	Legacy    *afirmauri.Adaptador
	Firmar    SignDocumentUseCase
	Flujo     RequestHandler
	LegacyRaw LegacyMessageHandler
	Trust     ports.TrustPolicy
}

// New construye el adaptador WebSocket heredado.
func New(legacy *afirmauri.Adaptador, firmar SignDocumentUseCase, flujo ...RequestHandler) *Adaptador {
	var handler RequestHandler
	if len(flujo) > 0 {
		handler = flujo[0]
	}
	return &Adaptador{
		Legacy: legacy,
		Firmar: firmar,
		Flujo:  handler,
	}
}

// WithLegacyHandler configura un procesador de respuestas legacy V1.
func (a *Adaptador) WithLegacyHandler(handler LegacyMessageHandler) *Adaptador {
	if a == nil {
		return nil
	}
	a.LegacyRaw = handler
	return a
}

// WithTrustPolicy comparte con el servidor WebSocket la misma politica de
// confianza que usa el parser/orquestador de firma.
func (a *Adaptador) WithTrustPolicy(policy ports.TrustPolicy) *Adaptador {
	if a == nil {
		return nil
	}
	a.Trust = policy
	return a
}

// HandleText procesa un unico mensaje textual heredado.
func (a *Adaptador) HandleText(ctx context.Context, origin, message string) (resultado Resultado, err error) {
	trace := legacyWebSocketAdapterTraceLogger()
	defer func() {
		if r := recover(); r != nil {
			trace.ErrorContext(ctx, "legacy_adapter_panic", "origin", origin, "panic", r, "stack", string(debug.Stack()))
			resultado = Resultado{}
			err = fmt.Errorf("panic en el flujo legacy websocket/service: %v", r)
		}
	}()
	message = strings.TrimSpace(message)
	trace.DebugContext(ctx, "legacy_adapter_handle_start", "origin", origin, "message_len", len(message), "message_prefix", truncateForTrace(message, 220))
	if message == "" {
		trace.WarnContext(ctx, "legacy_adapter_empty_message")
		return Resultado{}, errors.New("el mensaje websocket no puede estar vacio")
	}

	if strings.HasPrefix(strings.ToLower(message), EchoRequestPrefix) {
		trace.DebugContext(ctx, "legacy_adapter_echo")
		return Resultado{
			Tipo:  "echo",
			Texto: EchoResponseOK,
		}, nil
	}

	if !strings.HasPrefix(strings.ToLower(message), "afirma://") {
		trace.WarnContext(ctx, "legacy_adapter_unsupported_message", "message_prefix", truncateForTrace(message, 220))
		return Resultado{}, errors.New("mensaje websocket heredado no soportado")
	}
	if a == nil || a.Legacy == nil {
		trace.ErrorContext(ctx, "legacy_adapter_missing_parser")
		return Resultado{}, errors.New("adaptador afirmauri no configurado")
	}

	solicitud, err := a.Legacy.ParseSocket(ctx, message)
	if err != nil {
		trace.WarnContext(ctx, "legacy_adapter_parse_error", "message_prefix", truncateForTrace(message, 220), "error", err)
		return Resultado{}, err
	}
	if origin = strings.TrimSpace(origin); origin != "" {
		solicitud.Origenes = append(solicitud.Origenes, origin)
		if err := a.Legacy.ValidateOrigins(ctx, solicitud.Origenes); err != nil {
			trace.WarnContext(ctx, "legacy_adapter_origin_rejected", "origin", origin, "error", err)
			return Resultado{}, err
		}
	}
	trace.DebugContext(
		ctx,
		"legacy_adapter_parse_ok",
		"operation",
		string(solicitud.Operacion),
		"request_id",
		maskLegacyTraceValue(solicitud.Sesion.RequestID),
		"has_sign",
		solicitud.SignCommand != nil,
		"has_retrieve",
		solicitud.RetrieveCommand != nil,
		"has_batch",
		solicitud.BatchCommand != nil,
		"has_remote_batch",
		solicitud.RemoteBatch != nil,
	)
	resultado = Resultado{
		Tipo:      "afirmauri",
		Operacion: solicitud.Operacion,
		Solicitud: &solicitud,
	}

	if a.LegacyRaw != nil {
		trace.DebugContext(ctx, "legacy_adapter_handler_dispatch", "operation", string(solicitud.Operacion))
		out, handled, err := a.LegacyRaw.HandleLegacy(ctx, message, solicitud)
		if err != nil {
			trace.WarnContext(ctx, "legacy_adapter_handler_error", "operation", string(solicitud.Operacion), "error", err)
			return Resultado{}, err
		}
		if handled {
			trace.DebugContext(ctx, "legacy_adapter_handler_handled", "operation", string(solicitud.Operacion), "result_type", out.Tipo, "requires_exchange", out.RequiereIntercambio)
			return out, nil
		}
	}

	if a.Flujo != nil {
		if err := a.Flujo.HandleRequest(ctx, solicitud); err != nil {
			trace.WarnContext(ctx, "legacy_adapter_flow_error", "operation", string(solicitud.Operacion), "error", err)
			return Resultado{}, err
		}
		resultado.Texto = EchoResponseOK
		resultado.RequiereIntercambio = false
		trace.DebugContext(ctx, "legacy_adapter_flow_ok", "operation", string(solicitud.Operacion))
		return resultado, nil
	}

	if solicitud.SignCommand == nil {
		resultado.RequiereIntercambio = solicitud.RetrieveCommand != nil
		trace.DebugContext(ctx, "legacy_adapter_no_sign_command", "operation", string(solicitud.Operacion), "requires_exchange", resultado.RequiereIntercambio)
		return resultado, nil
	}
	if a.Firmar == nil {
		trace.ErrorContext(ctx, "legacy_adapter_missing_sign_usecase", "operation", string(solicitud.Operacion))
		return Resultado{}, errors.New("caso de uso de firma no configurado")
	}

	firma, err := a.Firmar.Execute(ctx, *solicitud.SignCommand)
	if err != nil {
		trace.WarnContext(ctx, "legacy_adapter_sign_error", "operation", string(solicitud.Operacion), "error", err)
		return Resultado{}, err
	}
	resultado.Tipo = "firma"
	resultado.Texto = EchoResponseOK
	resultado.FirmaBase64 = base64.StdEncoding.EncodeToString(firma.Result.Data)
	resultado.Formato = string(firma.Result.Format)
	resultado.CertificateID = firma.CertificateUsed.ID
	trace.DebugContext(ctx, "legacy_adapter_sign_ok", "operation", string(solicitud.Operacion), "format", resultado.Formato, "signature_len", len(resultado.FirmaBase64))
	return resultado, nil
}

// FormatearRespuesta reduce el resultado a un mensaje textual legacy.
func FormatearRespuesta(resultado Resultado) string {
	if strings.TrimSpace(resultado.Texto) != "" {
		return resultado.Texto
	}
	if strings.TrimSpace(resultado.FirmaBase64) != "" {
		return resultado.FirmaBase64
	}
	if resultado.RequiereIntercambio {
		return "PENDING_REMOTE"
	}
	return fmt.Sprintf("OK:%s", resultado.Tipo)
}

func legacyWebSocketAdapterTraceLogger() *slog.Logger {
	return logging.New("legacy/websocket-adapter", os.Stderr)
}
