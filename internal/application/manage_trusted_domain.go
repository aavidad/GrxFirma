// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/ports"
)

// ManageTrustedDomainUseCase gestiona la politica de confianza para un origen.
type ManageTrustedDomainUseCase struct {
	politica ports.TrustPolicy
	auditor  *AuditUseCase
	eventos  ports.EventPublisher
}

// NuevoManageTrustedDomainUseCase construye el caso de uso.
func NuevoManageTrustedDomainUseCase(
	politica ports.TrustPolicy,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *ManageTrustedDomainUseCase {
	return &ManageTrustedDomainUseCase{
		politica: politica,
		auditor:  auditor,
		eventos:  eventos,
	}
}

// Ejecutar aplica la accion indicada sobre el origen.
func (uc *ManageTrustedDomainUseCase) Ejecutar(ctx context.Context, cmd ManageTrustedDomainCommand) (ManageTrustedDomainResult, error) {
	if err := ctx.Err(); err != nil {
		return ManageTrustedDomainResult{}, err
	}
	if uc == nil || uc.politica == nil {
		return ManageTrustedDomainResult{}, errors.New("politica de confianza no configurada")
	}
	origin := strings.TrimSpace(cmd.Origin)
	if origin == "" {
		return ManageTrustedDomainResult{}, errors.New("el origen no puede estar vacio")
	}

	var err error
	switch cmd.Action {
	case TrustActionAllow:
		err = uc.politica.Allow(ctx, origin)
	case TrustActionDeny:
		err = uc.politica.Deny(ctx, origin)
	case TrustActionRemove:
		err = uc.politica.Remove(ctx, origin)
	default:
		return ManageTrustedDomainResult{}, fmt.Errorf("accion de confianza no soportada: %s", cmd.Action)
	}
	if err != nil {
		uc.auditar(ctx, origin, cmd.Action, false, err)
		return ManageTrustedDomainResult{}, fmt.Errorf("no se pudo aplicar la accion de confianza: %w", err)
	}

	_ = uc.publicarEvento(ctx, "dominio_confianza_actualizado", fmt.Sprintf("%s:%s", cmd.Action, origin))
	uc.auditar(ctx, origin, cmd.Action, true, nil)
	return ManageTrustedDomainResult{
		Origin: origin,
		Action: cmd.Action,
	}, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *ManageTrustedDomainUseCase) Execute(ctx context.Context, cmd ManageTrustedDomainCommand) (ManageTrustedDomainResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *ManageTrustedDomainUseCase) auditar(ctx context.Context, origin string, action TrustAction, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "dominio_confianza",
		Origin:        origin,
		DocumentName:  string(action),
		Success:       success,
		ErrorSummary:  summary,
	})
}

func (uc *ManageTrustedDomainUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}
