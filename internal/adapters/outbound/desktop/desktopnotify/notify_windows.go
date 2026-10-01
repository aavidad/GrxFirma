// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package desktopnotify

import (
	"context"

	"grxfirma/internal/ports"
)

// Notificador implementa ports.DesktopNotification en Windows.
// Usa la API Toast de Windows 10+ vía golang.org/x/sys/windows.
// Por ahora es un stub funcional; la implementación completa va en T065.
type Notificador struct{}

// New crea un Notificador Windows.
func New() *Notificador { return &Notificador{} }

// NewConBinario existe por compatibilidad con la API Linux; se ignora en Windows.
func NewConBinario(_ string) *Notificador { return &Notificador{} }

// Notify muestra una notificación Toast en Windows.
// Implementación actual: no-op. Extender en T065 con syscall real.
func (n *Notificador) Notify(_ context.Context, _, _ string) error {
	return nil
}

var _ ports.DesktopNotification = (*Notificador)(nil)
