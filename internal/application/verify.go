// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"fmt"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type detachedVerifierEngine interface {
	VerifyDetached(ctx context.Context, signedDocument domain.Document, originalDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
}

// VerifySignatureUseCase orquesta la verificacion de un documento firmado.
type VerifySignatureUseCase struct {
	anclas   ports.TrustAnchorProvider
	verifier ports.VerifierEngine
	auditor  *AuditUseCase
}

// NuevoVerifySignatureUseCase construye el caso de uso de verificacion.
func NuevoVerifySignatureUseCase(
	anclas ports.TrustAnchorProvider,
	verifier ports.VerifierEngine,
	auditor *AuditUseCase,
) *VerifySignatureUseCase {
	return &VerifySignatureUseCase{
		anclas:   anclas,
		verifier: verifier,
		auditor:  auditor,
	}
}

// Ejecutar verifica el documento firmado y devuelve el resultado tecnico y los firmantes hallados.
func (uc *VerifySignatureUseCase) Ejecutar(ctx context.Context, cmd VerifyCommand) (VerifyResult, error) {
	anchors, err := uc.anclas.Anchors(ctx)
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return VerifyResult{}, fmt.Errorf("no se pudieron obtener los anclajes de confianza: %w", err)
	}

	var verification domain.VerificationResult
	var signers []domain.CertificateRef
	if cmd.OriginalDocument != nil {
		if detached, ok := uc.verifier.(detachedVerifierEngine); ok {
			verification, signers, err = detached.VerifyDetached(ctx, cmd.SignedDocument, *cmd.OriginalDocument, anchors)
		} else {
			err = fmt.Errorf("el verificador no soporta contenido original para firmas detached")
		}
	} else {
		verification, signers, err = uc.verifier.Verify(ctx, cmd.SignedDocument, anchors)
	}
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return VerifyResult{}, fmt.Errorf("no se pudo verificar la firma del documento: %w", err)
	}
	verification = verification.WithSignerSummaries(signers).Normalize()

	uc.auditar(ctx, cmd, verification.Valid, nil)
	return VerifyResult{
		Verification: verification,
		Firmantes:    signers,
	}, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *VerifySignatureUseCase) Execute(ctx context.Context, cmd VerifyCommand) (VerifyResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *VerifySignatureUseCase) auditar(ctx context.Context, cmd VerifyCommand, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	resumen := ""
	if err != nil {
		resumen = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "verificacion",
		DocumentName:  cmd.SignedDocument.Name,
		DocumentData:  cmd.SignedDocument.Content,
		Success:       success,
		ErrorSummary:  resumen,
	})
}
