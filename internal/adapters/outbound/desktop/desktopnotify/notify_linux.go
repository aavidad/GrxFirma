// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package desktopnotify

import (
	"context"
	"os/exec"
	"time"

	"grxfirma/internal/ports"
)

// Notificador implementa ports.DesktopNotification en Linux usando notify-send.
type Notificador struct {
	// notifySend es el path al binario notify-send; vacío = busca en $PATH.
	notifySend string
}

// New crea un Notificador Linux.
func New() *Notificador {
	return &Notificador{notifySend: "notify-send"}
}

// NewConBinario permite inyectar el path del binario para tests.
func NewConBinario(path string) *Notificador {
	return &Notificador{notifySend: path}
}

// Notify envía una notificación de escritorio. La llamada es no bloqueante:
// el subproceso notify-send se lanza en segundo plano. Si notify-send no está
// disponible o falla, retorna nil (la notificación es decorativa).
func (n *Notificador) Notify(ctx context.Context, title, body string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Timeout generoso: notify-send raramente tarda más de 2s.
	notifyCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// #nosec G204 -- notifySend is the fixed notify-send tool or an explicit
	// test dependency; title/body follow -- and are never shell input.
	cmd := exec.CommandContext(notifyCtx,
		n.notifySend,
		"--app-name=GrxFirma",
		"--urgency=normal",
		"--expire-time=5000",
		"--",
		title,
		body,
	)

	// No bloqueamos esperando el resultado: la notificación es best-effort.
	// Si notify-send falla la operación ya terminó correctamente.
	_ = cmd.Start()
	return nil
}

var _ ports.DesktopNotification = (*Notificador)(nil)
