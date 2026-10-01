// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	stdx509 "crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
	cryptobinx509 "github.com/deatil/go-cryptobin/x509"

	"grxfirma/internal/adapters/outbound/common/certutil"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

const (
	maxCMSSignedEnvelopedBytes        = 64 << 20
	maxCMSSignedEnvelopedSigners      = 16
	maxCMSSignedEnvelopedCertificates = 64
)

var (
	// ErrCMSSignedEnvelopedIntegrity identifica una firma o contenido alterado.
	ErrCMSSignedEnvelopedIntegrity = errors.New("integridad criptografica de SignedAndEnvelopedData no valida")
	// ErrCMSSignedEnvelopedValidity identifica un certificado fuera de vigencia
	// o no autorizado para firma digital.
	ErrCMSSignedEnvelopedValidity = errors.New("vigencia del firmante de SignedAndEnvelopedData no valida")
	// ErrCMSSignedEnvelopedTrust identifica una cadena que no alcanza las
	// anclas X.509 configuradas.
	ErrCMSSignedEnvelopedTrust = errors.New("confianza X.509 del firmante de SignedAndEnvelopedData no valida")
)

type CMSSignedEnvelopedProtector struct {
	clock ports.Clock
}

func NuevoCMSSignedEnvelopedProtector() *CMSSignedEnvelopedProtector {
	return &CMSSignedEnvelopedProtector{}
}

// WithClock inyecta el reloj del preflight. No se configura desde peticiones.
func (p *CMSSignedEnvelopedProtector) WithClock(clock ports.Clock) *CMSSignedEnvelopedProtector {
	p.clock = clock
	return p
}

func (p *CMSSignedEnvelopedProtector) ProtectAndSign(ctx context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient, key ports.SigningKey) (domain.ProtectedPayload, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProtectedPayload{}, err
	}
	if err := job.Validate(); err != nil {
		return domain.ProtectedPayload{}, err
	}
	if job.Profile != domain.ProtectionProfileCompat {
		return domain.ProtectedPayload{}, errors.New("SignedAndEnvelopedData solo esta disponible para el perfil compat")
	}
	if len(recipients) == 0 {
		return domain.ProtectedPayload{}, errors.New("debe existir al menos un destinatario de proteccion")
	}

	local, err := extractLocalSigningKey(key)
	if err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("clave de firma no exportable para SignedAndEnvelopedData: %w", err)
	}
	if local == nil || local.Certificate == nil || local.Signer == nil {
		return domain.ProtectedPayload{}, &signingpolicy.Error{Reason: signingpolicy.MissingIdentity}
	}
	now := time.Now()
	if p.clock != nil {
		now = p.clock.Now()
	}
	if err := signingpolicy.ValidateCertificateDER(local.Certificate.Raw, now); err != nil {
		return domain.ProtectedPayload{}, err
	}
	if err := validateCMSSigningIdentity(local.Certificate, local.Signer.Public()); err != nil {
		return domain.ProtectedPayload{}, err
	}

	signerCert, err := cryptobinx509.ParseCertificate(local.Certificate.Raw)
	if err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("certificado del firmante no valido para CMS firmado: %w", err)
	}
	cipher := cryptobinpkcs7.AES256GCM
	saed, err := cryptobinpkcs7.NewSignedAndEnvelopedData(job.Document.Content, cipher)
	if err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("creando estructura SignedAndEnvelopedData: %w", err)
	}
	saed.SetDigestAlgorithm(cryptobinpkcs7.OidDigestAlgorithmSHA256)
	if err := saed.AddSigner(signerCert, local.Signer); err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("anadiendo firmante CMS: %w", err)
	}
	for _, cert := range local.Chain {
		if cert == nil || len(cert.Raw) == 0 {
			continue
		}
		parent, err := cryptobinx509.ParseCertificate(cert.Raw)
		if err != nil {
			return domain.ProtectedPayload{}, fmt.Errorf("certificado intermedio del firmante no valido: %w", err)
		}
		saed.AddCertificate(parent)
	}

	recipientCount := 0
	for _, recipient := range recipients {
		recipientCert, err := parseCMSRecipientCertificate(recipient, job.Profile)
		if err != nil {
			return domain.ProtectedPayload{}, err
		}
		cert, err := cryptobinx509.ParseCertificate(recipientCert.Raw)
		if err != nil {
			return domain.ProtectedPayload{}, fmt.Errorf("certificado del destinatario '%s' no valido para CMS firmado: %w", recipient.ID, err)
		}
		if err := saed.AddRecipient(cert); err != nil {
			return domain.ProtectedPayload{}, fmt.Errorf("anadiendo destinatario '%s' a SignedAndEnvelopedData: %w", recipient.ID, err)
		}
		recipientCount++
	}

	blob, err := saed.Finish()
	if err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("finalizando SignedAndEnvelopedData: %w", err)
	}
	protectedDoc, err := domain.NewDocument(protectedSignedCMSFileName(job.Document.Name), blob, domain.MIMETypeProtectedCMS)
	if err != nil {
		return domain.ProtectedPayload{}, err
	}
	return domain.ProtectedPayload{
		Document:       protectedDoc,
		Profile:        job.Profile,
		RecipientCount: recipientCount,
	}, nil
}

func decryptSignedEnvelopedCMS(ctx context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial, trustAnchors ports.TrustAnchorProvider) (domain.UnprotectedPayload, error) {
	if err := ctx.Err(); err != nil {
		return domain.UnprotectedPayload{}, err
	}
	if len(protected.Content) == 0 || len(protected.Content) > maxCMSSignedEnvelopedBytes {
		return domain.UnprotectedPayload{}, errors.New("tamano de SignedAndEnvelopedData no permitido")
	}
	if legacy, detected, err := decryptLegacyV1SignedEnvelopedCMS(ctx, protected, keys, trustAnchors); detected {
		return legacy, err
	}

	var (
		p7  *cryptobinpkcs7.PKCS7
		err error
	)
	func() {
		cmsCryptobinParseMu.Lock()
		defer cmsCryptobinParseMu.Unlock()
		p7, err = cryptobinpkcs7.Parse(protected.Content)
	}()
	if err != nil {
		return domain.UnprotectedPayload{}, fmt.Errorf("contenedor SignedAndEnvelopedData no valido: %w", err)
	}
	embeddedCerts, signerCerts, err := extractCMSSignedEnvelopedCertificates(p7)
	if err != nil {
		return domain.UnprotectedPayload{}, err
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return domain.UnprotectedPayload{}, err
		}
		if !key.Supports(domain.ProtectionProfileCompat) {
			continue
		}
		if len(key.CertificateDER) == 0 || len(key.RSAOAEP256PrivateKeyPKCS8) == 0 {
			continue
		}
		stdCert, priv, err := parseCMSRSAKeyMaterial(key)
		if err != nil {
			continue
		}
		cert, err := cryptobinx509.ParseCertificate(stdCert.Raw)
		if err != nil {
			continue
		}
		if err := p7.Decrypt(cert, priv); err != nil {
			continue
		}
		// Verify() sin truststore comprueba exclusivamente la integridad
		// matemática. No se presenta como validación de confianza X.509.
		if err := p7.Verify(); err != nil {
			zeroBytes(p7.Content)
			p7.Content = nil
			return domain.UnprotectedPayload{}, fmt.Errorf("%w: %v", ErrCMSSignedEnvelopedIntegrity, err)
		}
		if err := validateCMSSignedEnvelopedSignerValidity(signerCerts, time.Now()); err != nil {
			zeroBytes(p7.Content)
			p7.Content = nil
			return domain.UnprotectedPayload{}, err
		}
		if err := validateCMSSignedEnvelopedSignerTrust(ctx, trustAnchors, signerCerts, embeddedCerts); err != nil {
			zeroBytes(p7.Content)
			p7.Content = nil
			return domain.UnprotectedPayload{}, err
		}
		doc, err := domain.NewDocument(unprotectedCMSFileName(protected.Name), p7.Content, inferUnprotectedMIME(protected.MIMEType))
		if err != nil {
			zeroBytes(p7.Content)
			p7.Content = nil
			return domain.UnprotectedPayload{}, err
		}
		return domain.UnprotectedPayload{
			Document:    doc,
			Profile:     domain.ProtectionProfileCompat,
			RecipientID: key.RecipientID,
		}, nil
	}
	return domain.UnprotectedPayload{}, errors.New("no se pudo desproteger el SignedAndEnvelopedData con las claves disponibles")
}

func extractCMSSignedEnvelopedCertificates(p7 *cryptobinpkcs7.PKCS7) ([]*stdx509.Certificate, []*stdx509.Certificate, error) {
	if p7 == nil ||
		len(p7.Signers) == 0 || len(p7.Signers) > maxCMSSignedEnvelopedSigners ||
		len(p7.Certificates) == 0 || len(p7.Certificates) > maxCMSSignedEnvelopedCertificates {
		return nil, nil, errors.New("cardinalidad de firmantes o certificados SignedAndEnvelopedData no permitida")
	}
	embedded := make([]*stdx509.Certificate, 0, len(p7.Certificates))
	for _, compatCert := range p7.Certificates {
		if compatCert == nil || len(compatCert.Raw) == 0 || len(compatCert.Raw) > maxCMSCertificateDERBytes {
			return nil, nil, errors.New("certificado SignedAndEnvelopedData fuera de limites")
		}
		cert, err := stdx509.ParseCertificate(compatCert.Raw)
		if err != nil {
			return nil, nil, errors.New("certificado SignedAndEnvelopedData no valido")
		}
		embedded = append(embedded, cert)
	}
	signers := make([]*stdx509.Certificate, 0, len(p7.Signers))
	for _, signer := range p7.Signers {
		issuerAndSerial := signer.IssuerAndSerialNumber
		if issuerAndSerial.SerialNumber == nil || issuerAndSerial.SerialNumber.Sign() < 0 ||
			len(issuerAndSerial.IssuerName.FullBytes) == 0 {
			return nil, nil, errors.New("identificador de firmante SignedAndEnvelopedData no valido")
		}
		var found *stdx509.Certificate
		for _, cert := range embedded {
			if cert.SerialNumber.Cmp(issuerAndSerial.SerialNumber) == 0 &&
				bytes.Equal(cert.RawIssuer, issuerAndSerial.IssuerName.FullBytes) {
				found = cert
				break
			}
		}
		if found == nil {
			return nil, nil, errors.New("certificado del firmante SignedAndEnvelopedData no encontrado")
		}
		signers = append(signers, found)
	}
	return embedded, signers, nil
}

func validateCMSSignedEnvelopedSignerValidity(signers []*stdx509.Certificate, now time.Time) error {
	for _, cert := range signers {
		if cert == nil {
			return fmt.Errorf("%w: certificado firmante nulo", ErrCMSSignedEnvelopedValidity)
		}
		switch {
		case now.Before(cert.NotBefore):
			return fmt.Errorf("%w: certificado firmante aun no valido", ErrCMSSignedEnvelopedValidity)
		case now.After(cert.NotAfter):
			return fmt.Errorf("%w: certificado firmante caducado", ErrCMSSignedEnvelopedValidity)
		case cert.KeyUsage != 0 && cert.KeyUsage&(stdx509.KeyUsageDigitalSignature|stdx509.KeyUsageContentCommitment) == 0:
			return fmt.Errorf("%w: certificado firmante sin uso de firma digital", ErrCMSSignedEnvelopedValidity)
		}
	}
	return nil
}

func validateCMSSignedEnvelopedSignerTrust(ctx context.Context, provider ports.TrustAnchorProvider, signers, embedded []*stdx509.Certificate) error {
	if provider == nil {
		return fmt.Errorf("%w: proveedor de anclas no configurado", ErrCMSSignedEnvelopedTrust)
	}
	anchors, err := provider.Anchors(ctx)
	if err != nil {
		return fmt.Errorf("%w: no se pudieron obtener las anclas: %v", ErrCMSSignedEnvelopedTrust, err)
	}
	trust := commonsigner.EvaluateCertificateTrust(signers, embedded, anchors)
	if trust.Status != domain.VerificationStatusValid {
		reason := strings.TrimSpace(trust.Reason)
		if reason == "" {
			reason = "la cadena no alcanzo una ancla configurada"
		}
		return fmt.Errorf("%w: %s", ErrCMSSignedEnvelopedTrust, reason)
	}
	return nil
}

func validateCMSSigningIdentity(cert *stdx509.Certificate, publicKey any) error {
	if cert == nil || len(cert.Raw) == 0 {
		return errors.New("certificado del firmante CMS no disponible")
	}
	if len(cert.Raw) > maxCMSCertificateDERBytes {
		return errors.New("el certificado del firmante CMS excede el limite permitido")
	}
	if !certutil.PuedeDigitalmenteSign(cert) {
		return errors.New("el certificado del firmante CMS no permite firma digital")
	}
	certPublicDER, err := stdx509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return fmt.Errorf("clave publica del certificado firmante CMS no valida: %w", err)
	}
	signerPublicDER, err := stdx509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("clave publica del firmante CMS no valida: %w", err)
	}
	if !bytes.Equal(certPublicDER, signerPublicDER) {
		return errors.New("la clave de firma CMS no corresponde al certificado seleccionado")
	}
	return nil
}

func protectedSignedCMSFileName(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "documento"
	}
	return base + ".signedenveloped.p7m"
}

func trimSignedProtectedSuffix(trimmed string) string {
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".signedenveloped.p7m") {
		sinP7m := strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
		return strings.TrimSuffix(sinP7m, filepath.Ext(sinP7m))
	}
	return trimmed
}
