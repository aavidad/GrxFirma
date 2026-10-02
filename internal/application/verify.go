// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type detachedVerifierEngine interface {
	VerifyDetached(ctx context.Context, signedDocument domain.Document, originalDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
}

// VerifySignatureUseCase orquesta la verificacion de un documento firmado.
type VerifySignatureUseCase struct {
	anclas    ports.TrustAnchorProvider
	verifier  ports.VerifierEngine
	auditor   *AuditUseCase
	evaluador ports.EvaluadorDictamenFirma
	ahora     func() time.Time
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

// ConAnclas sustituye el proveedor de anclas de confianza, por ejemplo por
// un conjunto local fijado en configuración en lugar del almacén del sistema.
func (uc *VerifySignatureUseCase) ConAnclas(anclas ports.TrustAnchorProvider) *VerifySignatureUseCase {
	if uc == nil || anclas == nil {
		return uc
	}
	uc.anclas = anclas
	return uc
}

// ConEvaluador activa el dictamen explícito (cadena, revocación, sello y
// vínculo evaluados con fuentes locales). Sin evaluador, la respuesta
// conserva exactamente el contrato anterior.
func (uc *VerifySignatureUseCase) ConEvaluador(evaluador ports.EvaluadorDictamenFirma) *VerifySignatureUseCase {
	if uc == nil {
		return nil
	}
	uc.evaluador = evaluador
	return uc
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

	var dictamen *domain.DictamenVerificacion
	if uc.evaluador != nil {
		evaluado, err := uc.componerDictamen(ctx, cmd, verification, anchors)
		if err != nil {
			uc.auditar(ctx, cmd, false, err)
			return VerifyResult{}, fmt.Errorf("no se pudo evaluar el dictamen de la firma: %w", err)
		}
		dictamen = &evaluado
	}

	uc.auditar(ctx, cmd, verification.Valid, nil)
	return VerifyResult{
		Verification: verification,
		Firmantes:    signers,
		Dictamen:     dictamen,
	}, nil
}

// componerDictamen delega la evidencia en el evaluador y fija aquí, fuera del
// adaptador, las huellas de eco y el veredicto global.
func (uc *VerifySignatureUseCase) componerDictamen(ctx context.Context, cmd VerifyCommand, verification domain.VerificationResult, anchors domain.CertificateChain) (domain.DictamenVerificacion, error) {
	ahora := time.Now
	if uc.ahora != nil {
		ahora = uc.ahora
	}
	referencia := ahora().UTC()
	dictamen, err := uc.evaluador.Evaluar(ctx, ports.EntradaDictamen{
		Firmado:    cmd.SignedDocument,
		Original:   cmd.OriginalDocument,
		Resultado:  verification,
		Anclas:     anchors,
		Referencia: referencia,
	})
	if err != nil {
		return domain.DictamenVerificacion{}, err
	}
	dictamen.ComprobadoEn = referencia
	dictamen.Formato = verification.Format
	dictamen.HuellaFirmadoSHA256 = huellaSHA256(cmd.SignedDocument.Content)
	dictamen.HuellaOriginalSHA256 = ""
	if cmd.OriginalDocument != nil {
		dictamen.HuellaOriginalSHA256 = huellaSHA256(cmd.OriginalDocument.Content)
	}
	return dictamen.Componer(), nil
}

func huellaSHA256(contenido []byte) string {
	suma := sha256.Sum256(contenido)
	return hex.EncodeToString(suma[:])
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
