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

// UploadResultUseCase sube un resultado serializado a una sesion remota.
type UploadResultUseCase struct {
	transporte ports.ResultTransport
	auditor    *AuditUseCase
	eventos    ports.EventPublisher
}

// NuevoUploadResultUseCase construye el caso de uso UploadResult.
func NuevoUploadResultUseCase(
	transporte ports.ResultTransport,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *UploadResultUseCase {
	return &UploadResultUseCase{
		transporte: transporte,
		auditor:    auditor,
		eventos:    eventos,
	}
}

// Ejecutar sube los datos serializados al servidor remoto usando la sesion indicada.
func (uc *UploadResultUseCase) Ejecutar(ctx context.Context, cmd UploadResultCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil || uc.transporte == nil {
		return errors.New("transporte de resultados no configurado")
	}
	if err := cmd.Session.Validate(); err != nil {
		return fmt.Errorf("sesion de intercambio no valida: %w", err)
	}
	if len(cmd.Data) == 0 {
		return errors.New("los datos a subir no pueden estar vacios")
	}

	_ = uc.publicarEvento(ctx, "subida_resultado_iniciada", cmd.Session.RequestID)

	if err := uc.transporte.Upload(ctx, cmd.Session, cmd.Data); err != nil {
		uc.auditar(ctx, cmd, false, err)
		return fmt.Errorf("no se pudo subir el resultado remoto: %w", err)
	}

	_ = uc.publicarEvento(ctx, "subida_resultado_completada", cmd.Session.RequestID)
	uc.auditar(ctx, cmd, true, nil)
	return nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *UploadResultUseCase) Execute(ctx context.Context, cmd UploadResultCommand) error {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *UploadResultUseCase) auditar(ctx context.Context, cmd UploadResultCommand, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "subida_resultado",
		Origin:        cmd.Session.UploadEndpoint,
		DocumentName:  fmt.Sprintf("bytes=%d", len(cmd.Data)),
		DocumentData:  cmd.Data,
		Success:       success,
		ErrorSummary:  summary,
	})
}

func (uc *UploadResultUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}
