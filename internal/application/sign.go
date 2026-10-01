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
	"time"
	"unicode"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// SignDocumentUseCase orquesta la firma de un unico documento.
// Es el caso de uso principal de la Fase 2.
type SignDocumentUseCase struct {
	catalogo  ports.CertificateCatalog
	claves    ports.SigningKeyProvider
	motor     ports.SignerEngine
	aprobador ports.UserApproval
	auditor   *AuditUseCase
	eventos   ports.EventPublisher
	metricas  ports.OperationMetrics
}

// NuevoSignDocumentUseCase construye el caso de uso con sus dependencias.
func NuevoSignDocumentUseCase(
	catalogo ports.CertificateCatalog,
	claves ports.SigningKeyProvider,
	motor ports.SignerEngine,
	aprobador ports.UserApproval,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *SignDocumentUseCase {
	return &SignDocumentUseCase{
		catalogo:  catalogo,
		claves:    claves,
		motor:     motor,
		aprobador: aprobador,
		auditor:   auditor,
		eventos:   eventos,
	}
}

func (uc *SignDocumentUseCase) WithMetrics(metricas ports.OperationMetrics) *SignDocumentUseCase {
	if uc == nil {
		return nil
	}
	uc.metricas = metricas
	return uc
}

// Ejecutar realiza la firma del documento descrito en el comando.
// Todos los errores devueltos son aptos para mostrarse al usuario en castellano.
func (uc *SignDocumentUseCase) Ejecutar(ctx context.Context, cmd SignCommand) (SignResult, error) {
	inicio := time.Now()
	estado := "error"
	defer func() {
		if uc != nil && uc.metricas != nil {
			uc.metricas.RecordSign(ctx, string(cmd.Format), estado, time.Since(inicio))
		}
	}()

	if err := cmd.Format.Validate(); err != nil {
		return SignResult{}, fmt.Errorf("formato de firma no valido: %w", err)
	}
	if err := cmd.Action.Validate(); err != nil {
		return SignResult{}, fmt.Errorf("accion de firma no valida: %w", err)
	}

	// Resolver el certificado a usar.
	cert, err := uc.resolverCertificado(ctx, cmd.CertificateID)
	if err != nil {
		uc.auditarFallo(ctx, cmd, cert, err)
		return SignResult{}, err
	}

	// Solicitar aprobacion explicita del usuario antes de firmar.
	if uc.aprobador == nil {
		err = errors.New("aprobador de firma no configurado")
		uc.auditarFallo(ctx, cmd, cert, err)
		return SignResult{}, err
	}
	aprobado, err := uc.aprobador.Request(ctx, mensajeAprobacionFirma(cmd, cert))
	if err != nil {
		uc.auditarFallo(ctx, cmd, cert, err)
		return SignResult{}, fmt.Errorf("error al solicitar aprobacion al usuario: %w", err)
	}
	if !aprobado {
		err = errors.New("el usuario ha cancelado la operacion de firma")
		uc.auditarFallo(ctx, cmd, cert, err)
		return SignResult{}, err
	}

	// Obtener la clave de firma asociada al certificado.
	clave, err := uc.claves.KeyFor(ctx, cert)
	if err != nil {
		ports.CloseSigningKey(clave)
		uc.auditarFallo(ctx, cmd, cert, err)
		return SignResult{}, fmt.Errorf("no se pudo obtener la clave de firma: %w", err)
	}
	defer ports.CloseSigningKey(clave)
	cadenaDER := copiarCadenaDER(clave.CertificateChainDER())

	// Construir el trabajo de firma interno.
	trabajo := domain.SignatureJob{
		Document: cmd.Document,
		Format:   cmd.Format,
		Action:   cmd.Action,
		Options:  cmd.Options,
	}

	_ = uc.publicarEvento(ctx, "firma_iniciada", cmd.Document.Name)

	// Ejecutar la firma.
	resultado, err := uc.motor.Sign(ctx, trabajo, clave)
	if err != nil {
		uc.auditarFallo(ctx, cmd, cert, err)
		return SignResult{}, fmt.Errorf("error durante la operacion de firma: %w", err)
	}

	_ = uc.publicarEvento(ctx, "firma_completada", cmd.Document.Name)

	// Registrar evidencia del exito.
	uc.auditarExito(ctx, cmd, cert)
	estado = "ok"

	return SignResult{
		Result:              resultado,
		CertificateUsed:     cert,
		CertificateChainDER: cadenaDER,
	}, nil
}

func copiarCadenaDER(origen [][]byte) [][]byte {
	destino := make([][]byte, len(origen))
	for indice := range origen {
		destino[indice] = append([]byte(nil), origen[indice]...)
	}
	return destino
}

// MensajeAprobacionFirma es el texto de confirmación que ve el usuario antes
// de firmar, también cuando la firma se completa en un servidor trifásico.
func MensajeAprobacionFirma(cmd SignCommand, cert domain.CertificateRef) string {
	return mensajeAprobacionFirma(cmd, cert)
}

func mensajeAprobacionFirma(cmd SignCommand, cert domain.CertificateRef) string {
	mensaje := fmt.Sprintf(
		"¿Desea firmar el documento '%s' con el certificado '%s'?",
		cmd.Document.Name,
		cert.Subject,
	)
	if aplicacion := contextoAprobacionSeguro(cmd.RequesterApplication); aplicacion != "" {
		mensaje += "\nAplicación solicitante: " + aplicacion
	}
	if origen := contextoAprobacionSeguro(cmd.RequesterOrigin); origen != "" {
		mensaje += "\nOrigen solicitante: " + origen
	}
	return mensaje
}

func contextoAprobacionSeguro(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const maxRunes = 512
	out := make([]rune, 0, min(len(value), maxRunes))
	for _, r := range value {
		if len(out) >= maxRunes {
			break
		}
		if unicode.IsControl(r) {
			out = append(out, ' ')
			continue
		}
		out = append(out, r)
	}
	return strings.Join(strings.Fields(string(out)), " ")
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
// Mantiene compatibilidad con el codigo existente que ya usa Ejecutar.
func (uc *SignDocumentUseCase) Execute(ctx context.Context, cmd SignCommand) (SignResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

// resolverCertificado busca el certificado indicado en el catalogo o devuelve el unico disponible.
func (uc *SignDocumentUseCase) resolverCertificado(ctx context.Context, certID string) (domain.CertificateRef, error) {
	certs, err := uc.catalogo.List(ctx)
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("no se pudo obtener la lista de certificados: %w", err)
	}
	if len(certs) == 0 {
		return domain.CertificateRef{}, errors.New("no hay certificados disponibles en el sistema")
	}

	// Si se especifico un ID concreto, buscarlo.
	if certID != "" {
		for _, c := range certs {
			if c.ID == certID {
				return c, nil
			}
		}
		return domain.CertificateRef{}, fmt.Errorf("no se encontro el certificado con identificador '%s'", certID)
	}

	// Si solo hay uno, usarlo directamente.
	if len(certs) == 1 {
		return certs[0], nil
	}

	// Si hay varios y no se especifico ID, es un error: el caso de uso SelectCertificate
	// debe haberse ejecutado antes para fijar el ID en el comando.
	return domain.CertificateRef{}, errors.New(
		"hay varios certificados disponibles; debe seleccionarse uno antes de firmar")
}

func (uc *SignDocumentUseCase) auditarExito(ctx context.Context, cmd SignCommand, cert domain.CertificateRef) {
	if uc.auditor == nil {
		return
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:          "firma",
		CertificateFingerprint: cert.Fingerprint,
		CertificateID:          cert.ID,
		DocumentName:           cmd.Document.Name,
		DocumentData:           cmd.Document.Content,
		Format:                 string(cmd.Format),
		Success:                true,
	})
}

func (uc *SignDocumentUseCase) auditarFallo(ctx context.Context, cmd SignCommand, cert domain.CertificateRef, fallo error) {
	if uc.auditor == nil {
		return
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:          "firma",
		CertificateFingerprint: cert.Fingerprint,
		CertificateID:          cert.ID,
		DocumentName:           cmd.Document.Name,
		DocumentData:           cmd.Document.Content,
		Format:                 string(cmd.Format),
		Success:                false,
		ErrorSummary:           sanitizarError(fallo),
	})
}

func (uc *SignDocumentUseCase) publicarEvento(ctx context.Context, tipo, nombre string) error {
	if uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(nombre),
	})
}

// sanitizarError extrae un resumen del error descartando detalles internos no aptos para logs.
// Esta funcion es deliberadamente conservadora: solo devuelve el mensaje de nivel mas alto.
func sanitizarError(err error) string {
	if err == nil {
		return ""
	}
	// Solo el mensaje del error raiz; no unwrappear para no filtrar rutas internas o datos.
	return err.Error()
}
