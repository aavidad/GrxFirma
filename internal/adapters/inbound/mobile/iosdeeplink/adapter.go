// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package iosdeeplink

import (
	"context"
	"fmt"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	mobileinbound "grxfirma/internal/adapters/inbound/mobile"
)

// Adaptador representa el punto de entrada iOS basado en deep links.
type Adaptador struct {
	legacy *afirmauri.Adaptador
}

// New construye el adaptador iOS deeplink.
func New(legacy *afirmauri.Adaptador) *Adaptador {
	return &Adaptador{legacy: legacy}
}

// Handle traduce un deep link a una operación interna.
func (a Adaptador) Handle(ctx context.Context, raw string) (mobileinbound.Solicitud, error) {
	if a.legacy == nil {
		return mobileinbound.Solicitud{}, fmt.Errorf("ios-deeplink: adaptador legacy afirmauri no configurado")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return mobileinbound.Solicitud{}, fmt.Errorf("ios-deeplink: deep link vacio")
	}
	return mobileinbound.ParsearDeepLinkMobile(ctx, a.legacy, raw, "ios-deeplink")
}
