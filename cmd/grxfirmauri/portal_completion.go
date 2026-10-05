// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"sync"
	"time"
)

const portalCompletionDelay = 5 * time.Second

// portalFailureCloseDelay deja leer el aviso de cancelación o de fallo antes
// de cerrar el servicio. Antes la ventana quedaba abierta hasta pulsar
// «Cerrar» y atendía con retraso la siguiente petición del portal.
const portalFailureCloseDelay = 10 * time.Second

// portalCompletion mantiene el plazo asociado únicamente a la última entrega.
// El reloj inyectable permite probar el vencimiento sin esperas reales.
type portalCompletion struct {
	mu          sync.Mutex
	now         func() time.Time
	deadline    time.Time
	kind        string
	failed      bool
	displayed   bool
	onDelivered func(string)
}

func newPortalCompletion(now func() time.Time) *portalCompletion {
	return &portalCompletion{now: now, onDelivered: portalDeliveredUI}
}

func (p *portalCompletion) delivered(kind string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deadline = p.now().Add(portalCompletionDelay)
	p.kind = kind
	p.failed = false
	p.displayed = false
}

func (p *portalCompletion) received() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deadline = time.Time{}
	p.kind = ""
	p.failed = false
	p.displayed = false
}

func (p *portalCompletion) failure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deadline = p.now().Add(portalFailureCloseDelay)
	p.kind = ""
	p.failed = true
	p.displayed = false
}

// snapshot devuelve el plazo interno restante y si corresponde cerrar el servicio.
func (p *portalCompletion) snapshot() (kind string, seconds int, closeNow, failed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deadline.IsZero() {
		return "", 0, false, p.failed
	}
	remaining := p.deadline.Sub(p.now())
	if remaining <= 0 {
		return p.kind, 0, true, p.failed
	}
	return p.kind, int((remaining + time.Second - 1) / time.Second), false, p.failed
}

func (p *portalCompletion) expire(cancel func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deadline.IsZero() || p.now().Before(p.deadline) {
		return false
	}
	if cancel != nil {
		cancel()
	}
	return true
}

// display oculta la ventana tras una entrega, una sola vez por operación.
// El plazo de cierre permanece interno para admitir otra solicitud del portal.
// Un fallo o una cancelación no se ocultan: su aviso queda a la vista hasta
// que vence el plazo.
func (p *portalCompletion) display() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deadline.IsZero() || p.failed || p.kind == "" || p.displayed || !p.now().Before(p.deadline) {
		return
	}
	p.displayed = true
	// Serializa el aviso con received/failure: una entrega anterior no debe
	// ocultar la interfaz de una operación nueva.
	p.onDelivered(p.kind)
}
