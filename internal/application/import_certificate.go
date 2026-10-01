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

// ImportCertificateUseCase importa un certificado externo a traves del puerto correspondiente.
type ImportCertificateUseCase struct {
	importador ports.CertificateImporter
	auditor    *AuditUseCase
	eventos    ports.EventPublisher
}

// NuevoImportCertificateUseCase construye el caso de uso.
func NuevoImportCertificateUseCase(
	importador ports.CertificateImporter,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *ImportCertificateUseCase {
	return &ImportCertificateUseCase{
		importador: importador,
		auditor:    auditor,
		eventos:    eventos,
	}
}

// Ejecutar importa un certificado externo y devuelve su referencia opaca.
func (uc *ImportCertificateUseCase) Ejecutar(ctx context.Context, cmd ImportCertificateCommand) (ImportCertificateResult, error) {
	if err := ctx.Err(); err != nil {
		return ImportCertificateResult{}, err
	}
	if uc == nil || uc.importador == nil {
		return ImportCertificateResult{}, errors.New("importador de certificados no configurado")
	}
	if len(cmd.Data) == 0 {
		return ImportCertificateResult{}, errors.New("los datos del certificado no pueden estar vacios")
	}

	_ = uc.publicarEvento(ctx, "importacion_certificado_iniciada", fmt.Sprintf("bytes=%d", len(cmd.Data)))

	cert, err := uc.importador.Import(ctx, cmd.Data, cmd.Password)
	if err != nil {
		uc.auditar(ctx, cmd, false, "", err)
		return ImportCertificateResult{}, fmt.Errorf("no se pudo importar el certificado: %w", err)
	}
	if err := cert.Validate(); err != nil {
		uc.auditar(ctx, cmd, false, cert.ID, err)
		return ImportCertificateResult{}, fmt.Errorf("el certificado importado no es valido: %w", err)
	}

	_ = uc.publicarEvento(ctx, "importacion_certificado_completada", cert.ID)
	uc.auditar(ctx, cmd, true, cert.ID, nil)
	return ImportCertificateResult{Certificate: cert}, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *ImportCertificateUseCase) Execute(ctx context.Context, cmd ImportCertificateCommand) (ImportCertificateResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *ImportCertificateUseCase) auditar(ctx context.Context, cmd ImportCertificateCommand, success bool, certID string, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "importacion_certificado",
		CertificateID: certID,
		DocumentName:  fmt.Sprintf("bytes=%d", len(cmd.Data)),
		DocumentData:  cmd.Data,
		Success:       success,
		ErrorSummary:  summary,
	})
}

func (uc *ImportCertificateUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}
