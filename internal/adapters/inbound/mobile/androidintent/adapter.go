// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package androidintent

import (
	"context"
	"fmt"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	mobileinbound "grxfirma/internal/adapters/inbound/mobile"
)

// Adaptador representa el punto de entrada Android basado en Intent.
type Adaptador struct {
	legacy *afirmauri.Adaptador
}

// New construye el adaptador Android.
func New(legacy *afirmauri.Adaptador) *Adaptador {
	return &Adaptador{legacy: legacy}
}

// Handle traduce un intent móvil a una operación interna.
func (a Adaptador) Handle(ctx context.Context, action string, payload []byte) (mobileinbound.Solicitud, error) {
	return a.HandleShared(ctx, action, "", payload)
}

// HandleShared traduce un intent móvil a una operación interna preservando
// metadatos documentales ya resueltos por la capa nativa Android.
func (a Adaptador) HandleShared(ctx context.Context, action, descriptor string, payload []byte) (mobileinbound.Solicitud, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "android.intent.action.view", "view":
		return a.handleView(ctx, payload)
	case "android.intent.action.send", "android.intent.action.send_multiple", "send", "send_multiple":
		return mobileinbound.BuildSolicitudDocumentoCompartido("android-intent", descriptor, payload)
	default:
		return mobileinbound.Solicitud{}, fmt.Errorf("android-intent: accion no soportada: %s", action)
	}
}

// HandleSharedMultiple traduce ACTION_SEND_MULTIPLE a una solicitud de lote.
// La capa nativa Android sigue siendo responsable de resolver `content://`,
// permisos temporales y metadatos básicos de cada entrada.
func (a Adaptador) HandleSharedMultiple(_ context.Context, action string, entradas []mobileinbound.DocumentoCompartidoEntrada) (mobileinbound.Solicitud, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "android.intent.action.send_multiple", "send_multiple":
		return mobileinbound.BuildSolicitudDocumentosCompartidos("android-intent", entradas)
	default:
		return mobileinbound.Solicitud{}, fmt.Errorf("android-intent: accion multiple no soportada: %s", action)
	}
}

func (a Adaptador) handleView(ctx context.Context, payload []byte) (mobileinbound.Solicitud, error) {
	if a.legacy == nil {
		return mobileinbound.Solicitud{}, fmt.Errorf("android-intent: adaptador legacy afirmauri no configurado")
	}
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return mobileinbound.Solicitud{}, fmt.Errorf("android-intent: deep link vacio")
	}
	return mobileinbound.ParsearDeepLinkMobile(ctx, a.legacy, raw, "android-intent")
}
