// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package trustdialog implementa ports.TrustPolicy decorando una política base
// con un diálogo GUI (Fyne) para las decisiones TOFU de primer uso.
//
// Cuando la política base retorna TrustPending (origen desconocido), el diálogo
// pregunta al usuario con tres opciones:
//
//   - "Confiar siempre" — llama a base.Allow() para persistir la decisión.
//   - "Confiar esta vez" — permite la operación actual sin persistir.
//   - "Rechazar" — cancela la operación.
package trustdialog

import (
	"context"
	"errors"
	"sync"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// DecisionUnicaVez representa una decisión temporal (no persistida).
type DecisionUnicaVez int

const (
	ConfiarSiempre DecisionUnicaVez = iota // persistida vía Allow()
	ConfiarEstaVez                         // temporal, no persistida
	Rechazar                               // deniega la operación
)

// ErrOrigenRechazado es el error retornado cuando el usuario rechaza el origen.
var ErrOrigenRechazado = errors.New("origen rechazado por el usuario")

// TrustUIProvider es la abstracción del diálogo que se muestra al usuario.
// Implementaciones: FyneTrustDialog (producción) y HeadlessTrustPolicy (tests).
type TrustUIProvider interface {
	PedirDecision(ctx context.Context, origen string) (DecisionUnicaVez, error)
}

// DuracionConfiarEstaVez es lo que dura en memoria una decisión "Confiar esta
// vez": cubre las validaciones repetidas de una misma operación (parser,
// orquestador, cada mensaje del socket) sin volver a preguntar (A-05).
const DuracionConfiarEstaVez = 5 * time.Minute

// TrustDialogPolicy decora ports.TrustPolicy añadiendo GUI para origenes desconocidos.
type TrustDialogPolicy struct {
	base ports.TrustPolicy
	ui   TrustUIProvider
	now  func() time.Time

	mu       sync.Mutex
	temporal map[string]time.Time
	enCurso  map[string]*preguntaEnCurso
}

type preguntaEnCurso struct {
	hecho     chan struct{}
	respuesta DecisionUnicaVez
	err       error
}

// New crea un TrustDialogPolicy que usa base como política de fondo y ui para los diálogos.
func New(base ports.TrustPolicy, ui TrustUIProvider) *TrustDialogPolicy {
	return &TrustDialogPolicy{
		base:     base,
		ui:       ui,
		now:      time.Now,
		temporal: make(map[string]time.Time),
		enCurso:  make(map[string]*preguntaEnCurso),
	}
}

// Evaluate evalúa si el origen está autorizado. Si la política base retorna TrustPending,
// muestra el diálogo GUI y actúa según la decisión del usuario.
func (p *TrustDialogPolicy) Evaluate(ctx context.Context, origin string) (domain.TrustDecision, error) {
	decision, err := p.base.Evaluate(ctx, origin)
	if err != nil {
		return domain.TrustDecision{}, err
	}

	if decision.Status != domain.TrustPending {
		return decision, nil
	}

	// Origen desconocido: si el usuario ya dijo "Confiar esta vez" hace poco,
	// no se vuelve a preguntar. La política base se consulta antes, así que un
	// rechazo posterior o la desactivación del TOFU siguen mandando.
	if p.confianzaTemporalVigente(origin) {
		return domain.TrustDecision{Origin: origin, Status: domain.TrustAllowed}, nil
	}

	respuesta, err := p.preguntar(ctx, origin)
	if err != nil {
		return domain.TrustDecision{}, err
	}

	switch respuesta {
	case ConfiarSiempre:
		if err := p.base.Allow(ctx, origin); err != nil {
			return domain.TrustDecision{}, err
		}
		return domain.TrustDecision{Origin: origin, Status: domain.TrustAllowed}, nil

	case ConfiarEstaVez:
		// Temporal: no se persiste; se recuerda en memoria un rato corto.
		p.recordarTemporal(origin)
		return domain.TrustDecision{Origin: origin, Status: domain.TrustAllowed}, nil

	default: // Rechazar
		return domain.TrustDecision{Origin: origin, Status: domain.TrustDenied}, ErrOrigenRechazado
	}
}

func (p *TrustDialogPolicy) confianzaTemporalVigente(origin string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	hasta, ok := p.temporal[origin]
	if !ok {
		return false
	}
	if !p.now().Before(hasta) {
		delete(p.temporal, origin)
		return false
	}
	return true
}

func (p *TrustDialogPolicy) recordarTemporal(origin string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.temporal[origin] = p.now().Add(DuracionConfiarEstaVez)
}

// preguntar muestra un solo diálogo por origen aunque lleguen varias
// validaciones a la vez; las demás esperan y comparten la respuesta.
func (p *TrustDialogPolicy) preguntar(ctx context.Context, origin string) (DecisionUnicaVez, error) {
	p.mu.Lock()
	if llamada, ok := p.enCurso[origin]; ok {
		p.mu.Unlock()
		select {
		case <-llamada.hecho:
			return llamada.respuesta, llamada.err
		case <-ctx.Done():
			return Rechazar, ctx.Err()
		}
	}
	llamada := &preguntaEnCurso{hecho: make(chan struct{})}
	p.enCurso[origin] = llamada
	p.mu.Unlock()

	llamada.respuesta, llamada.err = p.ui.PedirDecision(ctx, origin)

	p.mu.Lock()
	delete(p.enCurso, origin)
	p.mu.Unlock()
	close(llamada.hecho)
	return llamada.respuesta, llamada.err
}

// Allow delega en la política base.
func (p *TrustDialogPolicy) Allow(ctx context.Context, origin string) error {
	return p.base.Allow(ctx, origin)
}

// Deny delega en la política base y olvida la confianza temporal.
func (p *TrustDialogPolicy) Deny(ctx context.Context, origin string) error {
	p.mu.Lock()
	delete(p.temporal, origin)
	p.mu.Unlock()
	return p.base.Deny(ctx, origin)
}

// Remove delega en la política base y olvida la confianza temporal.
func (p *TrustDialogPolicy) Remove(ctx context.Context, origin string) error {
	p.mu.Lock()
	delete(p.temporal, origin)
	p.mu.Unlock()
	return p.base.Remove(ctx, origin)
}

var _ ports.TrustPolicy = (*TrustDialogPolicy)(nil)
