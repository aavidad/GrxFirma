// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"

	"grxfirma/internal/ports"
)

// RetrieveRequestUseCase recupera una peticion remota pendiente.
type RetrieveRequestUseCase struct {
	transporte ports.ResultTransport
	auditor    *AuditUseCase
	eventos    ports.EventPublisher
}

// NuevoRetrieveRequestUseCase construye el caso de uso RetrieveRequest.
func NuevoRetrieveRequestUseCase(
	transporte ports.ResultTransport,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *RetrieveRequestUseCase {
	return &RetrieveRequestUseCase{
		transporte: transporte,
		auditor:    auditor,
		eventos:    eventos,
	}
}

// Ejecutar recupera la peticion remota usando la sesion indicada.
func (uc *RetrieveRequestUseCase) Ejecutar(ctx context.Context, cmd RetrieveRequestCommand) (RetrieveRequestResult, error) {
	if err := ctx.Err(); err != nil {
		return RetrieveRequestResult{}, err
	}
	if uc == nil || uc.transporte == nil {
		return RetrieveRequestResult{}, errors.New("transporte de resultados no configurado")
	}
	if err := cmd.Session.Validate(); err != nil {
		return RetrieveRequestResult{}, fmt.Errorf("sesion de intercambio no valida: %w", err)
	}

	_ = uc.publicarEvento(ctx, "recuperacion_peticion_iniciada", cmd.Session.RequestID)

	data, err := uc.transporte.Retrieve(ctx, cmd.Session)
	if err != nil {
		uc.auditar(ctx, cmd, nil, false, err)
		return RetrieveRequestResult{}, fmt.Errorf("no se pudo recuperar la peticion remota: %w", err)
	}
	if len(data) == 0 {
		err = errors.New("la peticion remota recuperada esta vacia")
		uc.auditar(ctx, cmd, nil, false, err)
		return RetrieveRequestResult{}, err
	}

	_ = uc.publicarEvento(ctx, "recuperacion_peticion_completada", cmd.Session.RequestID)
	uc.auditar(ctx, cmd, data, true, nil)
	return RetrieveRequestResult{
		Session: cmd.Session,
		Data:    data,
	}, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *RetrieveRequestUseCase) Execute(ctx context.Context, cmd RetrieveRequestCommand) (RetrieveRequestResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *RetrieveRequestUseCase) auditar(ctx context.Context, cmd RetrieveRequestCommand, data []byte, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "recuperacion_peticion",
		Origin:        cmd.Session.RetrieveEndpoint,
		DocumentName:  fmt.Sprintf("bytes=%d", len(data)),
		DocumentData:  data,
		Success:       success,
		ErrorSummary:  summary,
	})
}

func (uc *RetrieveRequestUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}
