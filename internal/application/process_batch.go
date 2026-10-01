// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ProcessBatchUseCase orquesta el procesado secuencial de un lote de trabajos.
type ProcessBatchUseCase struct {
	catalogo  ports.CertificateCatalog
	claves    ports.SigningKeyProvider
	motor     ports.SignerEngine
	aprobador ports.UserApproval
	auditor   *AuditUseCase
	eventos   ports.EventPublisher
	metricas  ports.OperationMetrics
}

// NuevoProcessBatchUseCase construye el caso de uso ProcessBatch.
func NuevoProcessBatchUseCase(
	catalogo ports.CertificateCatalog,
	claves ports.SigningKeyProvider,
	motor ports.SignerEngine,
	aprobador ports.UserApproval,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *ProcessBatchUseCase {
	return &ProcessBatchUseCase{
		catalogo:  catalogo,
		claves:    claves,
		motor:     motor,
		aprobador: aprobador,
		auditor:   auditor,
		eventos:   eventos,
	}
}

func (uc *ProcessBatchUseCase) WithMetrics(metricas ports.OperationMetrics) *ProcessBatchUseCase {
	if uc == nil {
		return nil
	}
	uc.metricas = metricas
	return uc
}

// Ejecutar procesa todos los trabajos del lote y devuelve errores parciales por indice.
func (uc *ProcessBatchUseCase) Ejecutar(ctx context.Context, cmd ProcessBatchCommand) (BatchResult, error) {
	if err := ctx.Err(); err != nil {
		return BatchResult{}, err
	}

	batch := domain.BatchJob{
		Jobs:    cmd.Jobs,
		Session: cmd.Session,
	}
	if err := batch.Validate(); err != nil {
		return BatchResult{}, fmt.Errorf("lote no valido: %w", err)
	}

	cert, err := uc.resolverCertificado(ctx, cmd.CertificateID)
	if err != nil {
		uc.auditarLote(ctx, cmd, domain.CertificateRef{}, false, err)
		return BatchResult{}, err
	}
	if err := uc.solicitarAprobacion(ctx, cert, len(cmd.Jobs)); err != nil {
		uc.auditarLote(ctx, cmd, cert, false, err)
		return BatchResult{}, err
	}

	clave, err := uc.claves.KeyFor(ctx, cert)
	if err != nil {
		ports.CloseSigningKey(clave)
		uc.auditarLote(ctx, cmd, cert, false, err)
		return BatchResult{}, fmt.Errorf("no se pudo obtener la clave de firma del lote: %w", err)
	}
	defer ports.CloseSigningKey(clave)

	_ = uc.publicarEvento(ctx, "lote_iniciado", fmt.Sprintf("trabajos=%d", len(cmd.Jobs)))

	resultado := BatchResult{
		Results: make([]SignResult, 0, len(cmd.Jobs)),
		Errores: make(map[int]error),
	}

	for i, job := range cmd.Jobs {
		if err := ctx.Err(); err != nil {
			resultado.Errores[i] = err
			break
		}

		_ = uc.publicarEvento(ctx, "trabajo_lote_iniciado", fmt.Sprintf("indice=%d", i))
		inicioTrabajo := time.Now()
		firma, err := uc.motor.Sign(ctx, job, clave)
		if err != nil {
			resultado.Errores[i] = fmt.Errorf("trabajo %d: %w", i, err)
			uc.auditarTrabajo(ctx, i, job, cert, false, err)
			if uc != nil && uc.metricas != nil {
				uc.metricas.RecordSign(ctx, string(job.Format), "error", time.Since(inicioTrabajo))
			}
			if cmd.StopOnError {
				break
			}
			continue
		}

		resultado.Results = append(resultado.Results, SignResult{
			Result:          firma,
			CertificateUsed: cert,
		})
		uc.auditarTrabajo(ctx, i, job, cert, true, nil)
		if uc != nil && uc.metricas != nil {
			uc.metricas.RecordSign(ctx, string(job.Format), "ok", time.Since(inicioTrabajo))
		}
		_ = uc.publicarEvento(ctx, "trabajo_lote_completado", fmt.Sprintf("indice=%d", i))
	}

	_ = uc.publicarEvento(ctx, "lote_completado", fmt.Sprintf("errores=%d", len(resultado.Errores)))
	uc.auditarLote(ctx, cmd, cert, resultado.TodosExitosos(), nil)
	return resultado, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *ProcessBatchUseCase) Execute(ctx context.Context, cmd ProcessBatchCommand) (BatchResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *ProcessBatchUseCase) resolverCertificado(ctx context.Context, certID string) (domain.CertificateRef, error) {
	certs, err := uc.catalogo.List(ctx)
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("no se pudo obtener el catalogo de certificados: %w", err)
	}
	if len(certs) == 0 {
		return domain.CertificateRef{}, errors.New("no hay certificados disponibles para procesar el lote")
	}
	if certID != "" {
		for _, cert := range certs {
			if cert.ID == certID {
				return cert, nil
			}
		}
		return domain.CertificateRef{}, fmt.Errorf("no se encontro el certificado con identificador '%s'", certID)
	}
	if len(certs) == 1 {
		return certs[0], nil
	}
	return domain.CertificateRef{}, errors.New("hay varios certificados disponibles; debe seleccionarse uno antes de procesar el lote")
}

func (uc *ProcessBatchUseCase) solicitarAprobacion(ctx context.Context, cert domain.CertificateRef, total int) error {
	if uc == nil || uc.aprobador == nil {
		return errors.New("aprobador del lote no configurado")
	}
	ok, err := uc.aprobador.Request(ctx,
		fmt.Sprintf("¿Desea procesar %d trabajos con el certificado '%s'?", total, cert.Subject))
	if err != nil {
		return fmt.Errorf("error al solicitar aprobacion del lote: %w", err)
	}
	if !ok {
		return errors.New("el usuario ha cancelado el procesado del lote")
	}
	return nil
}

func (uc *ProcessBatchUseCase) auditarTrabajo(ctx context.Context, index int, job domain.SignatureJob, cert domain.CertificateRef, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:          "lote.trabajo",
		CertificateFingerprint: cert.Fingerprint,
		CertificateID:          cert.ID,
		DocumentName:           job.Document.Name,
		DocumentData:           job.Document.Content,
		Format:                 string(job.Format),
		Success:                success,
		ErrorSummary:           fmt.Sprintf("indice=%d %s", index, summary),
	})
}

func (uc *ProcessBatchUseCase) auditarLote(ctx context.Context, cmd ProcessBatchCommand, cert domain.CertificateRef, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:          "lote",
		CertificateFingerprint: cert.Fingerprint,
		CertificateID:          cert.ID,
		DocumentName:           fmt.Sprintf("trabajos=%d", len(cmd.Jobs)),
		Success:                success,
		ErrorSummary:           summary,
	})
}

func (uc *ProcessBatchUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}
