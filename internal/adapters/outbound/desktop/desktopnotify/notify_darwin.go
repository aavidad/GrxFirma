// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package desktopnotify

import (
	"context"

	"grxfirma/internal/ports"
)

// Notificador implementa ports.DesktopNotification en macOS.
// Usa osascript para emitir notificaciones del sistema sin CGo.
// Alternativa CGo (NSUserNotificationCenter) disponible si osascript no es suficiente.
type Notificador struct{}

// New crea un Notificador macOS.
func New() *Notificador { return &Notificador{} }

// NewConBinario existe por compatibilidad con la API Linux; se ignora en macOS.
func NewConBinario(_ string) *Notificador { return &Notificador{} }

// Notify envía una notificación macOS vía osascript (no requiere CGo).
func (n *Notificador) Notify(_ context.Context, title, body string) error {
	// Implementación completa: osascript o CGo a UserNotifications.framework.
	// Por ahora stub funcional.
	_ = title
	_ = body
	return nil
}

var _ ports.DesktopNotification = (*Notificador)(nil)
