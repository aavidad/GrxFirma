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

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ProtectDocumentUseCase orquesta la protección/cifrado de un documento.
type ProtectDocumentUseCase struct {
	recipients ports.ProtectionRecipientCatalog
	engine     ports.ProtectorEngine
	aprobador  ports.UserApproval
	auditor    *AuditUseCase
	eventos    ports.EventPublisher
}

func NuevoProtectDocumentUseCase(
	recipients ports.ProtectionRecipientCatalog,
	engine ports.ProtectorEngine,
	aprobador ports.UserApproval,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *ProtectDocumentUseCase {
	return &ProtectDocumentUseCase{
		recipients: recipients,
		engine:     engine,
		aprobador:  aprobador,
		auditor:    auditor,
		eventos:    eventos,
	}
}

func (uc *ProtectDocumentUseCase) Ejecutar(ctx context.Context, cmd ProtectCommand) (ProtectResult, error) {
	if err := ctx.Err(); err != nil {
		return ProtectResult{}, err
	}
	job := domain.ProtectionJob{
		Document:     cmd.Document,
		Profile:      cmd.Profile,
		Options:      cmd.Options,
		SymmetricKey: cmd.SymmetricKey,
	}
	if err := job.Validate(); err != nil {
		return ProtectResult{}, fmt.Errorf("proteccion no valida: %w", err)
	}
	if requiresProtectionRecipients(cmd) && len(cmd.RecipientIDs) == 0 {
		return ProtectResult{}, errors.New("debe indicarse al menos un destinatario de proteccion")
	}
	if uc == nil || uc.recipients == nil {
		return ProtectResult{}, errors.New("catalogo de destinatarios de proteccion no configurado")
	}
	if uc.engine == nil {
		return ProtectResult{}, errors.New("motor de proteccion no configurado")
	}

	recipients := []domain.ProtectionRecipient{}
	if requiresProtectionRecipients(cmd) {
		var err error
		recipients, err = uc.recipients.Resolve(ctx, cmd.RecipientIDs)
		if err != nil {
			uc.auditar(ctx, cmd, false, err)
			return ProtectResult{}, fmt.Errorf("no se pudieron resolver los destinatarios de proteccion: %w", err)
		}
		if len(recipients) == 0 {
			err = errors.New("no se encontraron destinatarios de proteccion validos")
			uc.auditar(ctx, cmd, false, err)
			return ProtectResult{}, err
		}
	}

	if err := uc.solicitarAprobacion(ctx, job, len(recipients)); err != nil {
		uc.auditar(ctx, cmd, false, err)
		return ProtectResult{}, err
	}

	_ = uc.publicarEvento(ctx, "proteccion_iniciada", cmd.Document.Name)
	protected, err := uc.engine.Protect(ctx, job, recipients)
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return ProtectResult{}, fmt.Errorf("no se pudo proteger el documento: %w", err)
	}
	_ = uc.publicarEvento(ctx, "proteccion_completada", protected.Document.Name)
	uc.auditar(ctx, cmd, true, nil)
	return ProtectResult{Protected: protected}, nil
}

func (uc *ProtectDocumentUseCase) Execute(ctx context.Context, cmd ProtectCommand) (ProtectResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *ProtectDocumentUseCase) solicitarAprobacion(ctx context.Context, job domain.ProtectionJob, recipients int) error {
	if uc == nil || uc.aprobador == nil {
		return nil
	}
	message := fmt.Sprintf("¿Desea proteger el documento '%s' para %d destinatario(s) con el perfil '%s'?",
		job.Document.Name, recipients, job.Profile)
	if recipients == 0 && !requiresProtectionRecipients(ProtectCommand{Options: job.Options}) {
		message = fmt.Sprintf("¿Desea proteger el documento '%s' con secreto simétrico transitorio y el perfil '%s'?",
			job.Document.Name, job.Profile)
	}
	ok, err := uc.aprobador.Request(ctx,
		message)
	if err != nil {
		return fmt.Errorf("error al solicitar aprobacion de proteccion: %w", err)
	}
	if !ok {
		return errors.New("el usuario ha cancelado la operacion de proteccion")
	}
	return nil
}

func (uc *ProtectDocumentUseCase) auditar(ctx context.Context, cmd ProtectCommand, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	resumen := ""
	if err != nil {
		resumen = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "proteccion",
		DocumentName:  cmd.Document.Name,
		DocumentData:  cmd.Document.Content,
		Format:        string(cmd.Profile),
		Success:       success,
		ErrorSummary:  resumen,
	})
}

func (uc *ProtectDocumentUseCase) publicarEvento(ctx context.Context, tipo, nombre string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{Type: tipo, Payload: []byte(nombre)})
}

func requiresProtectionRecipients(cmd ProtectCommand) bool {
	container := normalizeProtectionContainerToken(cmd.Options["container"])
	switch container {
	case "cmsencrypted", "cmsencrypteddata", "encrypted", "encrypteddata":
		return false
	default:
		return true
	}
}

func normalizeProtectionContainerToken(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	return normalized
}
