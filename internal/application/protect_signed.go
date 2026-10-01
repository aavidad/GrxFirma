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

// ProtectAndSignDocumentUseCase orquesta la creación de un contenedor protegido
// y firmado del remitente sin mezclar este flujo con la protección simple.
type ProtectAndSignDocumentUseCase struct {
	catalogo   ports.CertificateCatalog
	claves     ports.SigningKeyProvider
	recipients ports.ProtectionRecipientCatalog
	engine     ports.SignedProtectorEngine
	aprobador  ports.UserApproval
	auditor    *AuditUseCase
	eventos    ports.EventPublisher
}

func NuevoProtectAndSignDocumentUseCase(
	catalogo ports.CertificateCatalog,
	claves ports.SigningKeyProvider,
	recipients ports.ProtectionRecipientCatalog,
	engine ports.SignedProtectorEngine,
	aprobador ports.UserApproval,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *ProtectAndSignDocumentUseCase {
	return &ProtectAndSignDocumentUseCase{
		catalogo:   catalogo,
		claves:     claves,
		recipients: recipients,
		engine:     engine,
		aprobador:  aprobador,
		auditor:    auditor,
		eventos:    eventos,
	}
}

func (uc *ProtectAndSignDocumentUseCase) Ejecutar(ctx context.Context, cmd ProtectAndSignCommand) (ProtectAndSignResult, error) {
	if err := ctx.Err(); err != nil {
		return ProtectAndSignResult{}, err
	}
	job := domain.ProtectionJob{
		Document: cmd.Document,
		Profile:  cmd.Profile,
		Options:  normalizeProtectAndSignOptions(cmd.Options),
	}
	if err := job.Validate(); err != nil {
		return ProtectAndSignResult{}, fmt.Errorf("proteccion firmada no valida: %w", err)
	}
	if cmd.Profile != domain.ProtectionProfileCompat {
		return ProtectAndSignResult{}, errors.New("SignedAndEnvelopedData solo esta disponible para el perfil compat")
	}
	if err := validateProtectAndSignContainer(job.Options); err != nil {
		return ProtectAndSignResult{}, err
	}
	if len(cmd.RecipientIDs) == 0 {
		return ProtectAndSignResult{}, errors.New("debe indicarse al menos un destinatario de proteccion")
	}
	if uc == nil || uc.catalogo == nil || uc.claves == nil || uc.recipients == nil || uc.engine == nil {
		return ProtectAndSignResult{}, errors.New("proteccion firmada no configurada")
	}

	cert, err := uc.resolverCertificado(ctx, cmd.CertificateID)
	if err != nil {
		uc.auditar(ctx, cmd, cert, false, err)
		return ProtectAndSignResult{}, err
	}
	key, err := uc.claves.KeyFor(ctx, cert)
	if err != nil {
		ports.CloseSigningKey(key)
		uc.auditar(ctx, cmd, cert, false, err)
		return ProtectAndSignResult{}, fmt.Errorf("no se pudo obtener la clave de firma: %w", err)
	}
	defer ports.CloseSigningKey(key)
	recipients, err := uc.recipients.Resolve(ctx, cmd.RecipientIDs)
	if err != nil {
		uc.auditar(ctx, cmd, cert, false, err)
		return ProtectAndSignResult{}, fmt.Errorf("no se pudieron resolver los destinatarios de proteccion: %w", err)
	}
	if len(recipients) == 0 {
		err = errors.New("no se encontraron destinatarios de proteccion validos")
		uc.auditar(ctx, cmd, cert, false, err)
		return ProtectAndSignResult{}, err
	}
	if err := uc.solicitarAprobacion(ctx, cmd, cert, len(recipients)); err != nil {
		uc.auditar(ctx, cmd, cert, false, err)
		return ProtectAndSignResult{}, err
	}

	_ = uc.publicarEvento(ctx, "proteccion_firmada_iniciada", cmd.Document.Name)
	protected, err := uc.engine.ProtectAndSign(ctx, job, recipients, key)
	if err != nil {
		uc.auditar(ctx, cmd, cert, false, err)
		return ProtectAndSignResult{}, fmt.Errorf("no se pudo proteger y firmar el documento: %w", err)
	}
	_ = uc.publicarEvento(ctx, "proteccion_firmada_completada", protected.Document.Name)
	uc.auditar(ctx, cmd, cert, true, nil)
	return ProtectAndSignResult{
		Protected:       protected,
		CertificateUsed: cert,
	}, nil
}

func (uc *ProtectAndSignDocumentUseCase) Execute(ctx context.Context, cmd ProtectAndSignCommand) (ProtectAndSignResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *ProtectAndSignDocumentUseCase) resolverCertificado(ctx context.Context, certID string) (domain.CertificateRef, error) {
	certs, err := uc.catalogo.List(ctx)
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("no se pudo obtener la lista de certificados: %w", err)
	}
	if len(certs) == 0 {
		return domain.CertificateRef{}, errors.New("no hay certificados disponibles en el sistema")
	}
	if certID != "" {
		for _, c := range certs {
			if c.ID == certID {
				return c, nil
			}
		}
		return domain.CertificateRef{}, fmt.Errorf("no se encontro el certificado con identificador '%s'", certID)
	}
	if len(certs) == 1 {
		return certs[0], nil
	}
	return domain.CertificateRef{}, errors.New("hay varios certificados disponibles; debe seleccionarse uno antes de proteger firmando")
}

func (uc *ProtectAndSignDocumentUseCase) solicitarAprobacion(ctx context.Context, cmd ProtectAndSignCommand, cert domain.CertificateRef, recipients int) error {
	if uc == nil || uc.aprobador == nil {
		return nil
	}
	ok, err := uc.aprobador.Request(ctx,
		fmt.Sprintf("¿Desea proteger y firmar el documento '%s' para %d destinatario(s) con el certificado '%s'?",
			cmd.Document.Name, recipients, cert.Subject))
	if err != nil {
		return fmt.Errorf("error al solicitar aprobacion de proteccion firmada: %w", err)
	}
	if !ok {
		return errors.New("el usuario ha cancelado la operacion de proteccion firmada")
	}
	return nil
}

func (uc *ProtectAndSignDocumentUseCase) auditar(ctx context.Context, cmd ProtectAndSignCommand, cert domain.CertificateRef, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	resumen := ""
	if err != nil {
		resumen = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:          "proteccion_firmada",
		CertificateFingerprint: cert.Fingerprint,
		CertificateID:          cert.ID,
		DocumentName:           cmd.Document.Name,
		DocumentData:           cmd.Document.Content,
		Format:                 string(cmd.Profile),
		Success:                success,
		ErrorSummary:           resumen,
	})
}

func (uc *ProtectAndSignDocumentUseCase) publicarEvento(ctx context.Context, tipo, nombre string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{Type: tipo, Payload: []byte(nombre)})
}

func normalizeProtectAndSignOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return map[string]string{"container": "signedandenvelopeddata"}
	}
	out := make(map[string]string, len(options))
	for k, v := range options {
		out[k] = v
	}
	out["container"] = normalizeProtectionContainerToken(out["container"])
	if strings.TrimSpace(out["container"]) == "" {
		out["container"] = "signedandenvelopeddata"
	}
	return out
}

func validateProtectAndSignContainer(options map[string]string) error {
	container := normalizeProtectionContainerToken(options["container"])
	switch container {
	case "", "cmssignedandenveloped", "signedandenveloped", "signedandenvelopeddata":
		return nil
	case "cmsauthenveloped", "authenveloped", "authenvelopeddata", "authenticatedenvelopeddata":
		return errors.New("AuthEnvelopedData de CMS heredado de AutoFirma 1.9 no esta soportado en proteger-firmando: solo se admite SignedAndEnvelopedData")
	case "cmsauthenticated", "cmsauthenticateddata", "authenticated", "authenticateddata":
		return errors.New("AuthenticatedData de CMS heredado de AutoFirma 1.9 no esta soportado en proteger-firmando: solo se admite SignedAndEnvelopedData")
	case "cmscompressed", "cmscompresseddata", "compressed", "compresseddata":
		return errors.New("CompressedData de CMS no esta soportado en proteger-firmando: solo se admite SignedAndEnvelopedData")
	case "cmsencrypted", "cmsencrypteddata", "encrypted", "encrypteddata":
		return errors.New("EncryptedData de CMS no corresponde al flujo proteger-firmando: use proteger con contenedor cms-encrypted")
	case "cms", "cmsenveloped", "cmsenvelopeddata", "enveloped", "envelopeddata":
		return errors.New("EnvelopedData de CMS no corresponde al flujo proteger-firmando: use proteger con contenedor cms")
	default:
		return fmt.Errorf("contenedor CMS no soportado en proteger-firmando: %q", container)
	}
}
