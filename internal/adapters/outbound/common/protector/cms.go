// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
	cryptobinx509 "github.com/deatil/go-cryptobin/x509"
	"github.com/digitorus/pkcs7"

	"grxfirma/internal/adapters/outbound/common/systemtrust"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// go-cryptobin convierte BER a DER mediante un contador global no protegido.
// Serializamos sus entradas de parseo hasta que la dependencia elimine ese
// estado global; el cifrado y el trabajo posterior sobre la estructura ya
// parseada no necesitan este mutex.
var cmsCryptobinParseMu sync.Mutex

const (
	protectionContainerOptionKey = "container"
	protectionContainerJSON      = "json"
	protectionContainerCMS       = "cms"
	protectionSecretOptionKey    = "secret_b64"
	protectionAES256KeyBytes     = 32
	maxCMSCertificateDERBytes    = 1 << 20
	maxCMSPublicKeyDERBytes      = 64 << 10
	maxCMSPrivateKeyPKCS8Bytes   = 1 << 20
)

type cmsContentType string

const (
	cmsContentTypeNone                cmsContentType = ""
	cmsContentTypeEnvelopedData       cmsContentType = "EnvelopedData"
	cmsContentTypeEncryptedData       cmsContentType = "EncryptedData"
	cmsContentTypeSignedEnvelopedData cmsContentType = "SignedAndEnvelopedData"
	cmsContentTypeAuthEnvelopedData   cmsContentType = "AuthEnvelopedData"
	cmsContentTypeAuthenticatedData   cmsContentType = "AuthenticatedData"
	cmsContentTypeCompressedData      cmsContentType = "CompressedData"
	cmsContentTypeUnknown             cmsContentType = "Unknown"
)

var (
	oidCMSEnvelopedData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 3}
	oidCMSSignedEnvelopedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 4}
	oidCMSEncryptedData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}
	oidCMSAuthenticatedData   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 2}
	oidCMSCompressedData      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 9}
	oidCMSAuthEnvelopedData   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 23}
)

var (
	errCMSSignedEnvelopedNotSupported = errors.New("SignedAndEnvelopedData de CMS heredado de AutoFirma 1.9 no se genera desde la protección simple en V2: requiere remitente firmante y se expone por el caso de uso separado proteger-firmando")
	errCMSAuthenticatedNotSupported   = errors.New("AuthenticatedData de CMS heredado de AutoFirma 1.9 detectado pero aun no soportado en V2: las dependencias Go CMS actuales (github.com/digitorus/pkcs7 y github.com/deatil/go-cryptobin/pkcs7) no implementan este modo autenticado de forma usable")
	errCMSCompressedNotSupported      = errors.New("CompressedData de CMS no se expone en V2: no aporta valor frente al contenedor actual y permanece fuera de alcance")
)

type cmsContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type CMSProtector struct {
	trustAnchors ports.TrustAnchorProvider
}

func NuevoCMSProtector() *CMSProtector {
	return NuevoCMSProtectorConAnclas(systemtrust.New())
}

// NuevoCMSProtectorConAnclas permite fijar explícitamente las anclas usadas al
// abrir contenedores CMS firmados. Un proveedor nulo conserva el fallo cerrado:
// nunca degrada SignedAndEnvelopedData a una mera comprobación matemática.
func NuevoCMSProtectorConAnclas(trustAnchors ports.TrustAnchorProvider) *CMSProtector {
	return &CMSProtector{trustAnchors: trustAnchors}
}

func (p *CMSProtector) Protect(_ context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient) (domain.ProtectedPayload, error) {
	if err := job.Validate(); err != nil {
		return domain.ProtectedPayload{}, err
	}
	if job.Profile != domain.ProtectionProfileCompat {
		return domain.ProtectedPayload{}, errors.New("el contenedor CMS solo esta disponible para el perfil compat")
	}
	contentType, err := requestedCMSContentType(job.Options)
	if err != nil {
		return domain.ProtectedPayload{}, err
	}
	if contentType != cmsContentTypeEncryptedData && len(recipients) == 0 {
		return domain.ProtectedPayload{}, errors.New("debe existir al menos un destinatario de proteccion")
	}
	switch contentType {
	case cmsContentTypeNone, cmsContentTypeEnvelopedData:
		// Operativo hoy.
	case cmsContentTypeEncryptedData:
		// Operativo hoy con secreto transitorio.
	case cmsContentTypeSignedEnvelopedData:
		return domain.ProtectedPayload{}, errCMSSignedEnvelopedNotSupported
	case cmsContentTypeAuthEnvelopedData:
		// Operativo hoy con AES-256-GCM y RSA-OAEP-SHA-256.
	case cmsContentTypeAuthenticatedData:
		return domain.ProtectedPayload{}, unsupportedAuthenticatedCMSModeError(contentType)
	case cmsContentTypeCompressedData:
		return domain.ProtectedPayload{}, errCMSCompressedNotSupported
	default:
		return domain.ProtectedPayload{}, errors.New("tipo CMS no soportado")
	}

	var (
		blob           []byte
		recipientCount int
	)
	switch contentType {
	case cmsContentTypeEncryptedData:
		secret, err := requiredProtectionSecret(
			job.Options,
			job.SymmetricKey,
		)
		if err != nil {
			return domain.ProtectedPayload{}, err
		}
		defer zeroBytes(secret)
		blob, err = cryptobinpkcs7.EncryptUsingPSK(rand.Reader, job.Document.Content, secret, cryptobinpkcs7.AES256GCM)
		if err != nil {
			return domain.ProtectedPayload{}, fmt.Errorf("creando CMS EncryptedData: %w", err)
		}
	case cmsContentTypeAuthEnvelopedData:
		blob, recipientCount, err = encryptCMSAuthEnvelopedData(job.Document.Content, recipients)
		if err != nil {
			return domain.ProtectedPayload{}, err
		}
	default:
		certs := make([]*cryptobinx509.Certificate, 0, len(recipients))
		for _, recipient := range recipients {
			cert, err := parseCMSRecipientCertificate(recipient, job.Profile)
			if err != nil {
				return domain.ProtectedPayload{}, err
			}
			compatCert, err := cryptobinx509.ParseCertificate(cert.Raw)
			if err != nil {
				return domain.ProtectedPayload{}, fmt.Errorf("certificado del destinatario '%s' no valido para CMS: %w", recipient.ID, err)
			}
			certs = append(certs, compatCert)
		}
		blob, err = cryptobinpkcs7.Encrypt(rand.Reader, job.Document.Content, certs, cryptobinpkcs7.Opts{
			Cipher:     cryptobinpkcs7.AES256GCM,
			KeyEncrypt: cryptobinpkcs7.KeyEncryptRSA,
			Mode:       cryptobinpkcs7.DefaultMode,
		})
		if err != nil {
			return domain.ProtectedPayload{}, fmt.Errorf("creando sobre CMS: %w", err)
		}
		recipientCount = len(certs)
	}
	protectedDoc, err := domain.NewDocument(protectedCMSFileName(job.Document.Name, contentType), blob, domain.MIMETypeProtectedCMS)
	if err != nil {
		return domain.ProtectedPayload{}, err
	}
	return domain.ProtectedPayload{
		Document:       protectedDoc,
		Profile:        job.Profile,
		RecipientCount: recipientCount,
	}, nil
}

func (p *CMSProtector) Unprotect(ctx context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
	if err := ctx.Err(); err != nil {
		return domain.UnprotectedPayload{}, err
	}
	contentType, err := detectCMSContentType(protected.Content)
	if err != nil {
		return domain.UnprotectedPayload{}, fmt.Errorf("contenedor CMS no valido: %w", err)
	}
	switch contentType {
	case cmsContentTypeEnvelopedData, cmsContentTypeEncryptedData:
		// Se sigue intentando con el parser estándar; EncryptedData se trata luego
		// para dar un error más claro si falta PSK.
	case cmsContentTypeSignedEnvelopedData:
		var anchors ports.TrustAnchorProvider
		if p != nil {
			anchors = p.trustAnchors
		}
		return decryptSignedEnvelopedCMS(ctx, protected, keys, anchors)
	case cmsContentTypeAuthEnvelopedData:
		return decryptCMSAuthEnvelopedData(ctx, protected, keys)
	case cmsContentTypeAuthenticatedData:
		return domain.UnprotectedPayload{}, errCMSAuthenticatedNotSupported
	case cmsContentTypeCompressedData:
		return domain.UnprotectedPayload{}, errCMSCompressedNotSupported
	}
	if contentType == cmsContentTypeEncryptedData {
		invalidSecretLength := false
		validSecretLength := false
		for _, key := range keys {
			if len(key.SymmetricKey) == 0 {
				continue
			}
			if len(key.SymmetricKey) != protectionAES256KeyBytes {
				invalidSecretLength = true
				continue
			}
			validSecretLength = true
			plaintext, err := decryptCMSEncryptedData(protected.Content, key.SymmetricKey)
			if err != nil {
				continue
			}
			doc, err := domain.NewDocument(unprotectedCMSFileName(protected.Name), plaintext, inferUnprotectedMIME(protected.MIMEType))
			if err != nil {
				return domain.UnprotectedPayload{}, err
			}
			return domain.UnprotectedPayload{
				Document:    doc,
				Profile:     domain.ProtectionProfileCompat,
				RecipientID: key.RecipientID,
			}, nil
		}
		if invalidSecretLength && !validSecretLength {
			return domain.UnprotectedPayload{}, errors.New("el secreto simetrico para CMS EncryptedData debe tener exactamente 32 bytes")
		}
		return domain.UnprotectedPayload{}, errors.New("no se pudo desproteger el CMS EncryptedData con el secreto simetrico proporcionado")
	}
	for _, key := range keys {
		if !key.Supports(domain.ProtectionProfileCompat) {
			continue
		}
		if len(key.CertificateDER) == 0 {
			continue
		}
		cert, priv, err := parseCMSRSAKeyMaterial(key)
		if err != nil {
			continue
		}
		plaintext, err := decryptCMSEnvelopedData(protected.Content, cert, priv)
		if err != nil {
			continue
		}
		doc, err := domain.NewDocument(unprotectedCMSFileName(protected.Name), plaintext, inferUnprotectedMIME(protected.MIMEType))
		if err != nil {
			return domain.UnprotectedPayload{}, err
		}
		return domain.UnprotectedPayload{
			Document:    doc,
			Profile:     domain.ProtectionProfileCompat,
			RecipientID: key.RecipientID,
		}, nil
	}
	return domain.UnprotectedPayload{}, errors.New("no se pudo desproteger el sobre CMS con las claves disponibles")
}

func requestedCMSContentType(options map[string]string) (cmsContentType, error) {
	if len(options) == 0 {
		return cmsContentTypeNone, nil
	}
	raw := normalizeProtectionContainerToken(options[protectionContainerOptionKey])
	switch raw {
	case "", protectionContainerJSON:
		return cmsContentTypeNone, nil
	case protectionContainerCMS, "cmsenveloped", "cmsenvelopeddata", "enveloped", "envelopeddata":
		return cmsContentTypeEnvelopedData, nil
	case "cmsencrypted", "cmsencrypteddata", "encrypted", "encrypteddata":
		return cmsContentTypeEncryptedData, nil
	case "cmssignedandenveloped", "signedandenveloped", "signedandenvelopeddata":
		return cmsContentTypeSignedEnvelopedData, nil
	case "cmsauthenveloped", "authenveloped", "authenvelopeddata", "authenticatedenvelopeddata":
		return cmsContentTypeAuthEnvelopedData, nil
	case "cmsauthenticated", "cmsauthenticateddata", "authenticated", "authenticateddata":
		return cmsContentTypeAuthenticatedData, nil
	case "cmscompressed", "cmscompresseddata", "compressed", "compresseddata":
		return cmsContentTypeCompressedData, nil
	default:
		return cmsContentTypeNone, fmt.Errorf("contenedor de proteccion no soportado: %q", raw)
	}
}

func normalizeProtectionContainerToken(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	return normalized
}

func detectCMSContentType(data []byte) (cmsContentType, error) {
	if len(data) == 0 {
		return cmsContentTypeUnknown, errors.New("contenido CMS vacio")
	}
	var info cmsContentInfo
	if _, err := asn1.Unmarshal(data, &info); err != nil {
		return cmsContentTypeUnknown, err
	}
	switch {
	case info.ContentType.Equal(oidCMSEnvelopedData):
		return cmsContentTypeEnvelopedData, nil
	case info.ContentType.Equal(oidCMSEncryptedData):
		return cmsContentTypeEncryptedData, nil
	case info.ContentType.Equal(oidCMSSignedEnvelopedData):
		return cmsContentTypeSignedEnvelopedData, nil
	case info.ContentType.Equal(oidCMSAuthEnvelopedData):
		return cmsContentTypeAuthEnvelopedData, nil
	case info.ContentType.Equal(oidCMSAuthenticatedData):
		return cmsContentTypeAuthenticatedData, nil
	case info.ContentType.Equal(oidCMSCompressedData):
		return cmsContentTypeCompressedData, nil
	default:
		return cmsContentTypeUnknown, fmt.Errorf("OID CMS no soportado: %v", info.ContentType)
	}
}

func unsupportedAuthenticatedCMSModeError(contentType cmsContentType) error {
	switch contentType {
	case cmsContentTypeAuthenticatedData:
		return errCMSAuthenticatedNotSupported
	default:
		return errors.New("modo CMS autenticado no soportado")
	}
}

func looksLikeJSONEnvelope(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return strings.HasPrefix(trimmed, "{")
}

func protectedCMSFileName(name string, contentType cmsContentType) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "documento"
	}
	if contentType == cmsContentTypeEncryptedData {
		return base + ".encrypted.p7m"
	}
	if contentType == cmsContentTypeAuthEnvelopedData {
		return base + ".authenveloped.p7m"
	}
	return base + ".enveloped"
}

func unprotectedCMSFileName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "documento"
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".enveloped") {
		return strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
	}
	if unwrapped := trimSignedProtectedSuffix(trimmed); unwrapped != trimmed {
		return unwrapped
	}
	if strings.HasSuffix(lower, ".encrypted.p7m") {
		sinP7m := strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
		return strings.TrimSuffix(sinP7m, filepath.Ext(sinP7m))
	}
	if strings.HasSuffix(lower, ".authenveloped.p7m") {
		sinP7m := strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
		return strings.TrimSuffix(sinP7m, filepath.Ext(sinP7m))
	}
	if strings.HasSuffix(lower, ".p7m") {
		return strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
	}
	return strings.TrimSuffix(trimmed, filepath.Ext(trimmed)) + "_desprotegido"
}

func inferUnprotectedMIME(protectedMIME string) string {
	if strings.EqualFold(strings.TrimSpace(protectedMIME), domain.MIMETypeProtectedCMS) {
		return "application/octet-stream"
	}
	return "application/octet-stream"
}

func requiredProtectionSecret(
	options map[string]string,
	symmetricKey []byte,
) ([]byte, error) {
	if len(symmetricKey) > 0 {
		if len(symmetricKey) != protectionAES256KeyBytes {
			return nil, errors.New("la clave simetrica transitoria debe contener exactamente 32 bytes para AES-256-GCM")
		}
		return append([]byte(nil), symmetricKey...), nil
	}
	secretB64 := options[protectionSecretOptionKey]
	if secretB64 == "" {
		return nil, errors.New("EncryptedData requiere secret_b64 con una clave AES-256-GCM transitoria codificada en Base64")
	}
	return decodeProtectionSecret(secretB64)
}

func decodeProtectionSecret(secretB64 string) ([]byte, error) {
	secret, err := base64.StdEncoding.Strict().DecodeString(secretB64)
	if err != nil {
		clear(secret)
		return nil, errors.New("secret_b64 no es valido")
	}
	if len(secret) != protectionAES256KeyBytes {
		clear(secret)
		return nil, errors.New("secret_b64 debe decodificar exactamente 32 bytes para AES-256-GCM")
	}

	// Strict rechaza padding bits distintos de cero; el roundtrip además fija
	// una única representación pública. Usamos un []byte controlable para no
	// crear otra string del secreto Base64 que no podríamos limpiar.
	canonical := make([]byte, base64.StdEncoding.EncodedLen(len(secret)))
	base64.StdEncoding.Encode(canonical, secret)
	isCanonical := len(canonical) == len(secretB64)
	for i := 0; isCanonical && i < len(canonical); i++ {
		isCanonical = canonical[i] == secretB64[i]
	}
	clear(canonical)
	if !isCanonical {
		clear(secret)
		return nil, errors.New("secret_b64 no es Base64 canonico")
	}

	// DecodeString ya devuelve una copia propiedad del llamador. Protect la
	// limpia con defer; no duplicarla aquí evita dejar otro backing array.
	return secret, nil
}

func parseCMSRecipientCertificate(recipient domain.ProtectionRecipient, profile domain.ProtectionProfile) (*x509.Certificate, error) {
	if err := recipient.Validate(profile); err != nil {
		return nil, err
	}
	if len(recipient.CertificateDER) == 0 {
		return nil, fmt.Errorf("el destinatario '%s' no expone certificado X.509 para CMS", recipient.ID)
	}
	if len(recipient.CertificateDER) > maxCMSCertificateDERBytes {
		return nil, fmt.Errorf("el certificado del destinatario '%s' excede el limite CMS", recipient.ID)
	}
	cert, err := x509.ParseCertificate(recipient.CertificateDER)
	if err != nil {
		return nil, fmt.Errorf("certificado del destinatario '%s' no valido: %w", recipient.ID, err)
	}
	certPublic, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("el destinatario '%s' no usa clave RSA compatible con CMS", recipient.ID)
	}
	// El transporte CMS actual es RSAES-PKCS1-v1_5: KeyAgreement no autoriza
	// por sí solo a usar el certificado para cifrar la CEK.
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		return nil, fmt.Errorf("el certificado del destinatario '%s' no permite cifrado de clave", recipient.ID)
	}
	if len(recipient.RSAOAEP256PublicKeyDER) > maxCMSPublicKeyDERBytes {
		return nil, fmt.Errorf("la clave publica declarada del destinatario '%s' excede el limite CMS", recipient.ID)
	}
	declaredPublicAny, err := x509.ParsePKIXPublicKey(recipient.RSAOAEP256PublicKeyDER)
	if err != nil {
		return nil, fmt.Errorf("clave publica declarada del destinatario '%s' no valida: %w", recipient.ID, err)
	}
	declaredPublic, ok := declaredPublicAny.(*rsa.PublicKey)
	if !ok || declaredPublic.N.Cmp(certPublic.N) != 0 || declaredPublic.E != certPublic.E {
		return nil, fmt.Errorf("la clave publica declarada del destinatario '%s' no coincide con su certificado CMS", recipient.ID)
	}
	return cert, nil
}

func parseCMSRSAKeyMaterial(key domain.ProtectionKeyMaterial) (*x509.Certificate, *rsa.PrivateKey, error) {
	if len(key.CertificateDER) == 0 || len(key.RSAOAEP256PrivateKeyPKCS8) == 0 {
		return nil, nil, errors.New("material CMS RSA incompleto")
	}
	if len(key.CertificateDER) > maxCMSCertificateDERBytes {
		return nil, nil, errors.New("el certificado CMS excede el limite permitido")
	}
	if len(key.RSAOAEP256PrivateKeyPKCS8) > maxCMSPrivateKeyPKCS8Bytes {
		return nil, nil, errors.New("la clave privada CMS excede el limite permitido")
	}
	cert, err := x509.ParseCertificate(key.CertificateDER)
	if err != nil {
		return nil, nil, fmt.Errorf("certificado CMS no valido: %w", err)
	}
	certPublic, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("el certificado CMS no contiene una clave RSA")
	}
	privAny, err := x509.ParsePKCS8PrivateKey(key.RSAOAEP256PrivateKeyPKCS8)
	if err != nil {
		return nil, nil, fmt.Errorf("clave privada CMS PKCS#8 no valida: %w", err)
	}
	priv, ok := privAny.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("la clave privada CMS no es RSA")
	}
	if err := priv.Validate(); err != nil {
		return nil, nil, fmt.Errorf("clave privada CMS RSA no valida: %w", err)
	}
	if priv.PublicKey.N.Cmp(certPublic.N) != 0 || priv.PublicKey.E != certPublic.E {
		return nil, nil, errors.New("la clave privada CMS no corresponde al certificado del destinatario")
	}
	return cert, priv, nil
}

func decryptCMSEnvelopedData(data []byte, cert *x509.Certificate, priv *rsa.PrivateKey) ([]byte, error) {
	compatCert, err := cryptobinx509.ParseCertificate(cert.Raw)
	if err == nil {
		var (
			plaintext  []byte
			decryptErr error
		)
		func() {
			cmsCryptobinParseMu.Lock()
			defer cmsCryptobinParseMu.Unlock()
			plaintext, decryptErr = cryptobinpkcs7.Decrypt(data, compatCert, crypto.PrivateKey(priv))
		}()
		if decryptErr == nil {
			return plaintext, nil
		}
	}

	// Compatibilidad de lectura con los contenedores V2 emitidos antes de
	// corregir la codificación DER de los parámetros AES-GCM.
	legacy, legacyErr := pkcs7.Parse(data)
	if legacyErr != nil {
		return nil, legacyErr
	}
	return legacy.Decrypt(cert, crypto.PrivateKey(priv))
}

func decryptCMSEncryptedData(data, secret []byte) ([]byte, error) {
	var (
		plaintext  []byte
		decryptErr error
	)
	func() {
		cmsCryptobinParseMu.Lock()
		defer cmsCryptobinParseMu.Unlock()
		plaintext, decryptErr = cryptobinpkcs7.DecryptUsingPSK(data, secret)
	}()
	if decryptErr == nil {
		return plaintext, nil
	}

	// Compatibilidad de lectura con los contenedores V2 emitidos antes de
	// corregir la codificación DER de los parámetros AES-GCM.
	legacy, err := pkcs7.Parse(data)
	if err != nil {
		return nil, err
	}
	return legacy.DecryptUsingPSK(secret)
}
