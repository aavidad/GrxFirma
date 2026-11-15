// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"grxfirma/internal/security/excepciones"
	"strconv"
	"strings"
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
	// confianzaForzada solo se compone en la CLI y en el escritorio: lo que
	// el usuario aceptó en su equipo (lote 14). Nil en REST, afirma://,
	// host nativo y móvil.
	confianzaForzada ports.AlmacenConfianzaForzada
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

// ConConfianzaForzada aplica lo que el usuario aceptó en su equipo para
// los cinco obstáculos forzables. Solo la CLI y el escritorio lo componen.
func (uc *VerifySignatureUseCase) ConConfianzaForzada(almacen ports.AlmacenConfianzaForzada) *VerifySignatureUseCase {
	if uc == nil {
		return nil
	}
	uc.confianzaForzada = almacen
	return uc
}

func (uc *VerifySignatureUseCase) verificar(ctx context.Context, cmd VerifyCommand, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if cmd.OriginalDocument != nil {
		if detached, ok := uc.verifier.(detachedVerifierEngine); ok {
			return detached.VerifyDetached(ctx, cmd.SignedDocument, *cmd.OriginalDocument, anchors)
		}
		return domain.VerificationResult{}, nil, fmt.Errorf("el verificador no soporta contenido original para firmas detached")
	}
	return uc.verifier.Verify(ctx, cmd.SignedDocument, anchors)
}

// Ejecutar verifica el documento firmado y devuelve el resultado tecnico y los firmantes hallados.
func (uc *VerifySignatureUseCase) Ejecutar(ctx context.Context, cmd VerifyCommand) (VerifyResult, error) {
	ctx, avisos := domain.ConAvisosOperacion(ctx)
	anchors, err := uc.anclas.Anchors(ctx)
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return VerifyResult{}, fmt.Errorf("no se pudieron obtener los anclajes de confianza: %w", err)
	}

	var registro *domain.RegistroConfianzaForzada
	if uc.confianzaForzada != nil {
		registro = domain.NuevoRegistroConfianzaForzada(uc.cargarConfianzaForzada(ctx))
	}
	verification, signers, err := uc.verificar(domain.ConRegistroConfianzaForzada(ctx, registro), cmd, anchors)
	if err != nil {
		var block *excepciones.Block
		if errors.As(err, &block) {
			uc.auditar(ctx, cmd, false, err)
			v := domain.VerificationResult{Valid: false, Reason: block.Error(), Coverage: domain.CoberturaNoAcreditada, Warnings: domain.UnirAvisos([]string{block.BlockKey}, avisos.Claves()...), Errors: []string{block.Error()}, Integrity: domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: block.Error()}, Trust: domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: block.Error()}}
			return VerifyResult{Verification: v.Normalize(), Firmantes: signers}, nil
		}
		uc.auditar(ctx, cmd, false, err)
		return VerifyResult{}, fmt.Errorf("no se pudo verificar la firma del documento: %w", err)
	}
	verification.Warnings = domain.UnirAvisos(verification.Warnings, avisos.Claves()...)
	verification = verification.WithSignerSummaries(signers).Normalize()
	forzada := uc.completarConfianzaForzada(ctx, cmd, anchors, registro, &verification)

	var dictamen *domain.DictamenVerificacion
	if uc.evaluador != nil {
		evaluado, err := uc.componerDictamen(ctx, cmd, verification, anchors)
		if err != nil {
			uc.auditar(ctx, cmd, false, err)
			return VerifyResult{}, fmt.Errorf("no se pudo evaluar el dictamen de la firma: %w", err)
		}
		verification.Warnings = domain.UnirAvisos(verification.Warnings, avisos.Claves()...)
		dictamen = &evaluado
	}

	uc.auditarConCompatibilidad(ctx, cmd, verification.Valid, nil, codigosConfianzaForzada(verification.Valid, forzada.Aplicada))
	return VerifyResult{
		Verification:     verification,
		Firmantes:        signers,
		Dictamen:         dictamen,
		ConfianzaForzada: forzada,
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
	dictamen = dictamen.Componer()
	if dictamen.Estado == domain.EstadoDictamenValida {
		if motivo := limiteDictamenClasico(verification); motivo != "" {
			dictamen.Estado = domain.EstadoDictamenIndeterminada
			dictamen.Motivo = motivo
		}
	}
	return dictamen, nil
}

// El dictamen de fuentes locales añade evidencia, pero nunca rehabilita una
// firma que el verificador del formato dejó sin validez.
func limiteDictamenClasico(v domain.VerificationResult) domain.MotivoDictamen {
	if !v.Valid {
		return domain.MotivoDictamenResultadoClasicoNoValido
	}
	forced := map[string]bool{}
	for _, e := range v.Evidence {
		if e.Type == domain.EvidenciaConfianzaForzada {
			forced[e.Summary] = true
		}
	}
	for _, e := range v.Evidence {
		if e.Type == domain.EvidenciaAlgoritmoAntiguo && !forced[string(domain.ConfianzaAlgoritmoSHA1)] {
			return domain.MotivoDictamenAlgoritmoHeredado
		}
	}
	for _, detail := range v.Certificate.Details {
		if bits, ok := strings.CutPrefix(detail, "rsa_bits="); ok {
			parsed, err := strconv.Atoi(bits)
			if err == nil && parsed < 2048 && !forced[string(domain.ConfianzaClaveRSACorta)] {
				return domain.MotivoDictamenClaveRSACorta
			}
		}
	}
	return ""
}

func huellaSHA256(contenido []byte) string {
	suma := sha256.Sum256(contenido)
	return hex.EncodeToString(suma[:])
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *VerifySignatureUseCase) Execute(ctx context.Context, cmd VerifyCommand) (VerifyResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

// EvaluarSelloDocumento conserva anclas y fuentes locales del dictamen v2.
func (uc *VerifySignatureUseCase) EvaluarSelloDocumento(ctx context.Context, token, contenido []byte, referencia time.Time) (domain.DictamenFirmante, domain.AspectoDictamen) {
	pendiente := domain.AspectoDictamen{Estado: domain.IntegridadParcial, Motivo: "sello_no_comprobado"}
	if uc == nil || uc.anclas == nil || uc.evaluador == nil {
		return domain.DictamenFirmante{}, pendiente
	}
	evaluador, ok := uc.evaluador.(ports.EvaluadorSelloDocumento)
	if !ok {
		return domain.DictamenFirmante{}, pendiente
	}
	anclas, err := uc.anclas.Anchors(ctx)
	if err != nil {
		return domain.DictamenFirmante{}, pendiente
	}
	return evaluador.EvaluarSelloDocumento(ctx, token, contenido, anclas, referencia)
}

func (uc *VerifySignatureUseCase) auditar(ctx context.Context, cmd VerifyCommand, success bool, err error) {
	uc.auditarConCompatibilidad(ctx, cmd, success, err, nil)
}

func (uc *VerifySignatureUseCase) auditarConCompatibilidad(ctx context.Context, cmd VerifyCommand, success bool, err error, compatibilidad []string) {
	if uc == nil || uc.auditor == nil {
		return
	}
	resumen := ""
	if err != nil {
		resumen = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:  "verificacion",
		DocumentName:   cmd.SignedDocument.Name,
		DocumentData:   cmd.SignedDocument.Content,
		Success:        success,
		ErrorSummary:   resumen,
		Compatibilidad: compatibilidad,
	})
}
