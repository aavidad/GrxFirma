// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !windows && !darwin

package desktopnotify

import (
	"context"

	"grxfirma/internal/ports"
)

// Notificador es el stub para plataformas sin implementación específica.
type Notificador struct{}

// New crea un Notificador stub (no-op).
func New() *Notificador { return &Notificador{} }

// NewConBinario existe por compatibilidad con tests multiplataforma.
func NewConBinario(_ string) *Notificador { return &Notificador{} }

// Notify es un no-op en plataformas sin soporte nativo de notificaciones.
func (n *Notificador) Notify(_ context.Context, _, _ string) error {
	return nil
}

var _ ports.DesktopNotification = (*Notificador)(nil)
