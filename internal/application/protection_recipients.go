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

type ExportProtectionRecipientUseCase struct {
	store ports.ProtectionRecipientExchangeStore
}

func NuevoExportProtectionRecipientUseCase(store ports.ProtectionRecipientExchangeStore) *ExportProtectionRecipientUseCase {
	return &ExportProtectionRecipientUseCase{store: store}
}

func (uc *ExportProtectionRecipientUseCase) Ejecutar(ctx context.Context, cmd ExportProtectionRecipientCommand) (ExportProtectionRecipientResult, error) {
	if err := ctx.Err(); err != nil {
		return ExportProtectionRecipientResult{}, err
	}
	if uc == nil || uc.store == nil {
		return ExportProtectionRecipientResult{}, errors.New("intercambio de destinatarios de protección no configurado")
	}
	recipientID := strings.TrimSpace(cmd.RecipientID)
	if recipientID == "" {
		return ExportProtectionRecipientResult{}, errors.New("debe indicar el identificador del destinatario a exportar")
	}
	recipient, data, err := uc.store.Export(ctx, recipientID)
	if err != nil {
		return ExportProtectionRecipientResult{}, fmt.Errorf("no se pudo exportar el destinatario de protección: %w", err)
	}
	return ExportProtectionRecipientResult{
		Recipient: recipient,
		Data:      data,
	}, nil
}

func (uc *ExportProtectionRecipientUseCase) Execute(ctx context.Context, cmd ExportProtectionRecipientCommand) (ExportProtectionRecipientResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

type ImportProtectionRecipientUseCase struct {
	store ports.ProtectionRecipientExchangeStore
}

func NuevoImportProtectionRecipientUseCase(store ports.ProtectionRecipientExchangeStore) *ImportProtectionRecipientUseCase {
	return &ImportProtectionRecipientUseCase{store: store}
}

func (uc *ImportProtectionRecipientUseCase) Ejecutar(ctx context.Context, cmd ImportProtectionRecipientCommand) (ImportProtectionRecipientResult, error) {
	if err := ctx.Err(); err != nil {
		return ImportProtectionRecipientResult{}, err
	}
	if uc == nil || uc.store == nil {
		return ImportProtectionRecipientResult{}, errors.New("intercambio de destinatarios de protección no configurado")
	}
	if len(cmd.Data) == 0 {
		return ImportProtectionRecipientResult{}, errors.New("el fichero de destinatario de protección no puede estar vacío")
	}
	recipient, err := uc.store.Import(ctx, cmd.Data)
	if err != nil {
		return ImportProtectionRecipientResult{}, fmt.Errorf("no se pudo importar el destinatario de protección: %w", err)
	}
	return ImportProtectionRecipientResult{Recipient: recipient}, nil
}

func (uc *ImportProtectionRecipientUseCase) Execute(ctx context.Context, cmd ImportProtectionRecipientCommand) (ImportProtectionRecipientResult, error) {
	return uc.Ejecutar(ctx, cmd)
}
