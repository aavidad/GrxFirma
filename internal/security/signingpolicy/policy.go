// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package signingpolicy comprueba la aptitud local antes de crear una firma.
// No valida confianza de cadena ni revocación: superar estas comprobaciones
// no acredita ninguna de ellas ni la aceptación por una sede.
package signingpolicy

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"time"
)

// ErrCertificateUnsuitable permite identificar un rechazo del preflight con errors.Is.
var ErrCertificateUnsuitable = errors.New("identidad no apta para crear una firma")

type Reason string

const (
	MissingIdentity      Reason = "missing_identity"
	InvalidCertificate   Reason = "invalid_certificate"
	NotYetValid          Reason = "not_yet_valid"
	Expired              Reason = "expired"
	CertificateAuthority Reason = "certificate_authority"
	DisallowedKeyUsage   Reason = "disallowed_key_usage"
	// DisallowedExtKeyUsage: los usos ampliados del certificado lo limitan a
	// otros fines (p. ej. solo servidor web HTTPS) y excluyen firmar documentos.
	DisallowedExtKeyUsage Reason = "disallowed_ext_key_usage"
)

// Error conserva el motivo estable sin incluir datos personales del certificado.
type Error struct {
	Reason Reason
}

func (e *Error) Error() string {
	switch e.Reason {
	case MissingIdentity:
		return "no hay una identidad de firma disponible; seleccione un certificado con clave privada"
	case InvalidCertificate:
		return "el certificado de firma no contiene un DER válido; vuelva a importar la identidad"
	case NotYetValid:
		return "el certificado de firma aún no está vigente; compruebe la fecha del equipo o seleccione otro certificado"
	case Expired:
		return "el certificado de firma está caducado; renueve o seleccione otro certificado"
	case CertificateAuthority:
		return "el certificado es de una autoridad de certificación; seleccione un certificado de usuario para firmar"
	case DisallowedKeyUsage:
		return "el certificado no permite firma digital ni compromiso de contenido; seleccione otro certificado"
	case DisallowedExtKeyUsage:
		return "el certificado solo sirve para otros fines (por ejemplo, servidores web HTTPS) y no para firmar documentos; seleccione otro certificado"
	default:
		return ErrCertificateUnsuitable.Error()
	}
}

func (e *Error) Unwrap() error { return ErrCertificateUnsuitable }

// ValidateCertificateDER usa exclusivamente los campos firmados del DER, nunca
// metadatos del selector ni opciones de una petición. Los extremos de vigencia
// son inclusivos. Una extensión KeyUsage ausente no impone restricciones de uso.
// now debe proceder del reloj del adaptador, no de parámetros del documento.
func ValidateCertificateDER(der []byte, now time.Time) error {
	if len(der) == 0 {
		return &Error{Reason: MissingIdentity}
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return &Error{Reason: InvalidCertificate}
	}
	if now.Before(cert.NotBefore) {
		return &Error{Reason: NotYetValid}
	}
	if now.After(cert.NotAfter) {
		return &Error{Reason: Expired}
	}
	if cert.IsCA {
		return &Error{Reason: CertificateAuthority}
	}
	for _, extension := range cert.Extensions {
		if extension.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 15}) &&
			cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0 {
			return &Error{Reason: DisallowedKeyUsage}
		}
	}
	if !usosAmpliadosPermitenFirma(cert) {
		return &Error{Reason: DisallowedExtKeyUsage}
	}
	return nil
}

// Usos ampliados compatibles con la firma de documentos. Sin extensión
// ExtendedKeyUsage no hay restricción (RFC 5280, 4.2.1.12); si existe, debe
// incluir alguno de estos.
var (
	oidFirmaDocumentos          = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 36}       // id-kp-documentSigning (RFC 9336)
	oidFirmaDocumentosMicrosoft = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 10, 3, 12} // Microsoft Document Signing
	oidDocumentosAutenticosPDF  = asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 5}       // Adobe Authentic Documents Trust
	usosAmpliadosFirma          = map[x509.ExtKeyUsage]bool{
		x509.ExtKeyUsageAny:             true,
		x509.ExtKeyUsageClientAuth:      true,
		x509.ExtKeyUsageEmailProtection: true,
		x509.ExtKeyUsageCodeSigning:     true,
	}
)

func usosAmpliadosPermitenFirma(cert *x509.Certificate) bool {
	if len(cert.ExtKeyUsage) == 0 && len(cert.UnknownExtKeyUsage) == 0 {
		return true
	}
	for _, uso := range cert.ExtKeyUsage {
		if usosAmpliadosFirma[uso] {
			return true
		}
	}
	for _, oid := range cert.UnknownExtKeyUsage {
		if oid.Equal(oidFirmaDocumentos) || oid.Equal(oidFirmaDocumentosMicrosoft) || oid.Equal(oidDocumentosAutenticosPDF) {
			return true
		}
	}
	return false
}
