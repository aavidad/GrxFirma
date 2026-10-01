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

// TrustDialogPolicy decora ports.TrustPolicy añadiendo GUI para origenes desconocidos.
type TrustDialogPolicy struct {
	base ports.TrustPolicy
	ui   TrustUIProvider
}

// New crea un TrustDialogPolicy que usa base como política de fondo y ui para los diálogos.
func New(base ports.TrustPolicy, ui TrustUIProvider) *TrustDialogPolicy {
	return &TrustDialogPolicy{base: base, ui: ui}
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

	// Origen desconocido: pedir decisión al usuario.
	respuesta, err := p.ui.PedirDecision(ctx, origin)
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
		// Temporal: no llamamos Allow(), solo retornamos Allowed para esta operación.
		return domain.TrustDecision{Origin: origin, Status: domain.TrustAllowed}, nil

	default: // Rechazar
		return domain.TrustDecision{Origin: origin, Status: domain.TrustDenied}, ErrOrigenRechazado
	}
}

// Allow delega en la política base.
func (p *TrustDialogPolicy) Allow(ctx context.Context, origin string) error {
	return p.base.Allow(ctx, origin)
}

// Deny delega en la política base.
func (p *TrustDialogPolicy) Deny(ctx context.Context, origin string) error {
	return p.base.Deny(ctx, origin)
}

// Remove delega en la política base.
func (p *TrustDialogPolicy) Remove(ctx context.Context, origin string) error {
	return p.base.Remove(ctx, origin)
}

var _ ports.TrustPolicy = (*TrustDialogPolicy)(nil)
