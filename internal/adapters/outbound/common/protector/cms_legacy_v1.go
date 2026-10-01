// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//lint:file-ignore SA1019 lector opt-in de RSA PKCS#1 v1.5 heredado; V2 nunca lo emite

package protector

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
)

const (
	legacyCMSAESOptInEnv       = "GRXFIRMA_ENABLE_LEGACY_CMS_AES_ECB"
	maxLegacyCMSRecipients     = 32
	maxLegacyCMSSigners        = 16
	maxLegacyCMSCertificates   = 64
	maxLegacyCMSCiphertextSize = 64 << 20
)

var (
	oidCMSData                   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidCMSRSAEncryption          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidCMSSHA256                 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidCMSSHA256WithRSA          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidCMSLegacyAESGeneric       = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1}
	oidCMSAttributeContentType   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidCMSAttributeMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
)

type legacyCMSIssuerAndSerial struct {
	IssuerName   asn1.RawValue
	SerialNumber *big.Int
}

type legacyCMSRecipientInfo struct {
	Version                int
	IssuerAndSerialNumber  legacyCMSIssuerAndSerial
	KeyEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedKey           []byte
}

type legacyCMSEncryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           asn1.RawValue `asn1:"optional,tag:0"`
}

type legacyCMSAttribute struct {
	Type  asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"set"`
}

type legacyCMSSignerInfo struct {
	Version                   int
	IssuerAndSerialNumber     legacyCMSIssuerAndSerial
	DigestAlgorithm           pkix.AlgorithmIdentifier
	AuthenticatedAttributes   []legacyCMSAttribute `asn1:"optional,omitempty,tag:0"`
	DigestEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedDigest           []byte
	UnauthenticatedAttributes []legacyCMSAttribute `asn1:"optional,omitempty,tag:1"`
}

// AutoFirma 1.9 etiquetaba los certificados de SignedAndEnvelopedData con [1]
// en vez del [0] esperado por las bibliotecas CMS actuales. Esta estructura
// existe solo para lectura de ese perfil histórico.
type legacyCMSSignedEnvelopedData struct {
	Version                    int
	RecipientInfos             []legacyCMSRecipientInfo   `asn1:"set"`
	DigestAlgorithmIdentifiers []pkix.AlgorithmIdentifier `asn1:"set"`
	EncryptedContentInfo       legacyCMSEncryptedContentInfo
	Certificates               asn1.RawValue         `asn1:"optional,tag:1"`
	SignerInfos                []legacyCMSSignerInfo `asn1:"set"`
}

// legacyCMSAESReaderEnabled: solo un administrador (política de máquina)
// puede habilitar la lectura de sobres AES-ECB heredados.
func legacyCMSAESReaderEnabled() bool {
	return machinepolicy.OptIn(machinepolicy.PermitirCMSAESECBLegacy)
}

func decryptLegacyV1SignedEnvelopedCMS(ctx context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial, trustAnchors ports.TrustAnchorProvider) (domain.UnprotectedPayload, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.UnprotectedPayload{}, true, err
	}
	envelope, certs, err := parseLegacyV1SignedEnvelopedCMS(protected.Content)
	if err != nil {
		return domain.UnprotectedPayload{}, false, nil
	}
	if !legacyCMSAESReaderEnabled() {
		return domain.UnprotectedPayload{}, true, fmt.Errorf(
			"SignedAndEnvelopedData AES-ECB heredado de AutoFirma 1.9 detectado; "+
				"su lectura esta deshabilitada por seguridad: un administrador debe habilitar la politica de maquina %s solo para migrar sobres de confianza",
			machinepolicy.PermitirCMSAESECBLegacy,
		)
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return domain.UnprotectedPayload{}, true, err
		}
		if !key.Supports(domain.ProtectionProfileCompat) {
			continue
		}
		cert, priv, err := parseCMSRSAKeyMaterial(key)
		if err != nil {
			continue
		}
		recipient := findLegacyCMSRecipient(envelope.RecipientInfos, cert)
		if recipient == nil {
			continue
		}
		candidates, err := decryptLegacyCMSContentCandidates(envelope.EncryptedContentInfo, recipient, priv)
		if err != nil {
			return domain.UnprotectedPayload{}, true, fmt.Errorf(
				"%w: clave de contenido o cifrado V1 no validos",
				ErrCMSSignedEnvelopedIntegrity,
			)
		}
		var verificationErr error
		for _, plaintext := range candidates {
			verificationErr = verifyLegacyCMSSigners(envelope.SignerInfos, certs, plaintext)
			if verificationErr != nil {
				zeroBytes(plaintext)
				continue
			}
			signerCerts, err := legacyCMSSignerCertificates(envelope.SignerInfos, certs)
			if err != nil {
				zeroBytes(plaintext)
				return domain.UnprotectedPayload{}, true, fmt.Errorf("%w: %v", ErrCMSSignedEnvelopedIntegrity, err)
			}
			if err := validateCMSSignedEnvelopedSignerValidity(signerCerts, time.Now()); err != nil {
				zeroBytes(plaintext)
				return domain.UnprotectedPayload{}, true, err
			}
			if err := validateCMSSignedEnvelopedSignerTrust(ctx, trustAnchors, signerCerts, certs); err != nil {
				zeroBytes(plaintext)
				return domain.UnprotectedPayload{}, true, err
			}
			doc, err := domain.NewDocument(
				unprotectedCMSFileName(protected.Name),
				plaintext,
				inferUnprotectedMIME(protected.MIMEType),
			)
			if err != nil {
				zeroBytes(plaintext)
				return domain.UnprotectedPayload{}, true, err
			}
			return domain.UnprotectedPayload{
				Document:    doc,
				Profile:     domain.ProtectionProfileCompat,
				RecipientID: key.RecipientID,
			}, true, nil
		}
		if verificationErr == nil {
			verificationErr = errors.New("padding o clave de contenido CMS V1 no validos")
		}
		return domain.UnprotectedPayload{}, true, fmt.Errorf("%w: %v", ErrCMSSignedEnvelopedIntegrity, verificationErr)
	}
	return domain.UnprotectedPayload{}, true, errors.New(
		"no se pudo desproteger el SignedAndEnvelopedData V1 con las claves disponibles",
	)
}

func legacyCMSSignerCertificates(signers []legacyCMSSignerInfo, certs []*x509.Certificate) ([]*x509.Certificate, error) {
	out := make([]*x509.Certificate, 0, len(signers))
	for _, signer := range signers {
		cert := findLegacyCMSSignerCertificate(certs, signer.IssuerAndSerialNumber)
		if cert == nil {
			return nil, errors.New("certificado del firmante CMS V1 no encontrado")
		}
		out = append(out, cert)
	}
	return out, nil
}

func parseLegacyV1SignedEnvelopedCMS(data []byte) (*legacyCMSSignedEnvelopedData, []*x509.Certificate, error) {
	if len(data) == 0 || len(data) > maxLegacyCMSCiphertextSize {
		return nil, nil, errors.New("tamano CMS V1 no permitido")
	}
	var info cmsContentInfo
	rest, err := asn1.Unmarshal(data, &info)
	if err != nil || len(rest) != 0 || !info.ContentType.Equal(oidCMSSignedEnvelopedData) {
		return nil, nil, errors.New("ContentInfo CMS V1 no valido")
	}
	var envelope legacyCMSSignedEnvelopedData
	rest, err = asn1.Unmarshal(info.Content.Bytes, &envelope)
	if err != nil || len(rest) != 0 {
		return nil, nil, errors.New("SignedAndEnvelopedData CMS V1 no valido")
	}
	if envelope.Version != 1 ||
		len(envelope.RecipientInfos) == 0 || len(envelope.RecipientInfos) > maxLegacyCMSRecipients ||
		len(envelope.DigestAlgorithmIdentifiers) == 0 ||
		len(envelope.DigestAlgorithmIdentifiers) > maxLegacyCMSSigners ||
		len(envelope.SignerInfos) == 0 || len(envelope.SignerInfos) > maxLegacyCMSSigners {
		return nil, nil, errors.New("cardinalidad CMS V1 no permitida")
	}
	for _, digestAlgorithm := range envelope.DigestAlgorithmIdentifiers {
		if !digestAlgorithm.Algorithm.Equal(oidCMSSHA256) ||
			!algorithmParametersAreNullOrAbsent(digestAlgorithm) {
			return nil, nil, errors.New("algoritmo de resumen CMS V1 no permitido")
		}
	}
	if (!envelope.EncryptedContentInfo.ContentType.Equal(oidCMSData) &&
		!envelope.EncryptedContentInfo.ContentType.Equal(oidCMSEncryptedData)) ||
		!envelope.EncryptedContentInfo.ContentEncryptionAlgorithm.Algorithm.Equal(oidCMSLegacyAESGeneric) ||
		!algorithmParametersAreNullOrAbsent(envelope.EncryptedContentInfo.ContentEncryptionAlgorithm) {
		return nil, nil, errors.New("algoritmo CMS V1 no reconocido")
	}
	encrypted := envelope.EncryptedContentInfo.EncryptedContent
	if encrypted.Class != 2 || encrypted.Tag != 0 || encrypted.IsCompound ||
		len(encrypted.Bytes) == 0 || len(encrypted.Bytes) > maxLegacyCMSCiphertextSize ||
		len(encrypted.Bytes)%aes.BlockSize != 0 {
		return nil, nil, errors.New("contenido cifrado CMS V1 no valido")
	}
	if envelope.Certificates.Class != 2 || envelope.Certificates.Tag != 1 ||
		!envelope.Certificates.IsCompound || len(envelope.Certificates.Bytes) == 0 {
		return nil, nil, errors.New("certificados CMS V1 no validos")
	}
	certs, err := x509.ParseCertificates(envelope.Certificates.Bytes)
	if err != nil || len(certs) == 0 || len(certs) > maxLegacyCMSCertificates {
		return nil, nil, errors.New("cadena de certificados CMS V1 no valida")
	}
	for _, cert := range certs {
		if len(cert.Raw) == 0 || len(cert.Raw) > maxCMSCertificateDERBytes {
			return nil, nil, errors.New("certificado CMS V1 fuera de limites")
		}
	}
	for _, recipient := range envelope.RecipientInfos {
		if recipient.Version != 0 ||
			recipient.IssuerAndSerialNumber.SerialNumber == nil ||
			recipient.IssuerAndSerialNumber.SerialNumber.Sign() < 0 ||
			!recipient.KeyEncryptionAlgorithm.Algorithm.Equal(oidCMSRSAEncryption) ||
			!algorithmParametersAreNullOrAbsent(recipient.KeyEncryptionAlgorithm) ||
			len(recipient.EncryptedKey) == 0 || len(recipient.EncryptedKey) > maxCMSPrivateKeyPKCS8Bytes {
			return nil, nil, errors.New("destinatario CMS V1 no valido")
		}
	}
	return &envelope, certs, nil
}

func algorithmParametersAreNullOrAbsent(algorithm pkix.AlgorithmIdentifier) bool {
	params := algorithm.Parameters
	if len(params.FullBytes) == 0 {
		return true
	}
	return params.Class == 0 && params.Tag == asn1.TagNull && len(params.Bytes) == 0
}

func findLegacyCMSRecipient(recipients []legacyCMSRecipientInfo, cert *x509.Certificate) *legacyCMSRecipientInfo {
	for i := range recipients {
		recipient := &recipients[i]
		if recipient.IssuerAndSerialNumber.SerialNumber.Cmp(cert.SerialNumber) == 0 &&
			bytes.Equal(recipient.IssuerAndSerialNumber.IssuerName.FullBytes, cert.RawIssuer) {
			return recipient
		}
	}
	return nil
}

func decryptLegacyCMSContentCandidates(info legacyCMSEncryptedContentInfo, recipient *legacyCMSRecipientInfo, priv *rsa.PrivateKey) ([][]byte, error) {
	if len(recipient.EncryptedKey) != priv.Size() {
		return nil, errors.New("clave cifrada CMS V1 no coincide con el destinatario")
	}
	ciphertext := info.EncryptedContent.Bytes
	candidates := make([][]byte, 0, 3)
	for _, keySize := range []int{16, 24, 32} {
		contentKey := make([]byte, keySize)
		if _, err := rand.Read(contentKey); err != nil {
			return nil, err
		}
		// El lector opt-in debe abrir RSA PKCS#1 v1.5 de V1.9; la variante de
		// clave de sesión evita revelar el padding.
		if err := rsa.DecryptPKCS1v15SessionKey(rand.Reader, priv, recipient.EncryptedKey, contentKey); err != nil {
			zeroBytes(contentKey)
			return nil, errors.New("clave cifrada CMS V1 no valida")
		}
		block, err := aes.NewCipher(contentKey)
		zeroBytes(contentKey)
		if err != nil {
			return nil, err
		}
		padded := make([]byte, len(ciphertext))
		for offset := 0; offset < len(ciphertext); offset += aes.BlockSize {
			block.Decrypt(padded[offset:offset+aes.BlockSize], ciphertext[offset:offset+aes.BlockSize])
		}
		plaintext, err := strictPKCS7Unpad(padded, aes.BlockSize)
		if err == nil {
			candidates = append(candidates, append([]byte(nil), plaintext...))
		}
		zeroBytes(padded)
	}
	return candidates, nil
}

func strictPKCS7Unpad(padded []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 || len(padded) == 0 || len(padded)%blockSize != 0 {
		return nil, errors.New("padding CMS V1 no valido")
	}
	paddingByte := padded[len(padded)-1]
	paddingSize := int(paddingByte)
	if paddingSize == 0 || paddingSize > blockSize || paddingSize > len(padded) {
		return nil, errors.New("padding CMS V1 no valido")
	}
	var mismatch byte
	for _, value := range padded[len(padded)-paddingSize:] {
		mismatch |= value ^ paddingByte
	}
	if subtle.ConstantTimeByteEq(mismatch, 0) != 1 {
		return nil, errors.New("padding CMS V1 no valido")
	}
	return padded[:len(padded)-paddingSize], nil
}

func verifyLegacyCMSSigners(signers []legacyCMSSignerInfo, certs []*x509.Certificate, plaintext []byte) error {
	if len(signers) == 0 {
		return errors.New("CMS V1 sin firmantes")
	}
	for _, signer := range signers {
		if signer.Version != 1 ||
			signer.IssuerAndSerialNumber.SerialNumber == nil ||
			len(signer.AuthenticatedAttributes) > 64 ||
			len(signer.UnauthenticatedAttributes) > 64 ||
			!signer.DigestAlgorithm.Algorithm.Equal(oidCMSSHA256) ||
			!algorithmParametersAreNullOrAbsent(signer.DigestAlgorithm) ||
			(!signer.DigestEncryptionAlgorithm.Algorithm.Equal(oidCMSRSAEncryption) &&
				!signer.DigestEncryptionAlgorithm.Algorithm.Equal(oidCMSSHA256WithRSA)) ||
			!algorithmParametersAreNullOrAbsent(signer.DigestEncryptionAlgorithm) ||
			len(signer.EncryptedDigest) == 0 || len(signer.EncryptedDigest) > maxCMSPrivateKeyPKCS8Bytes {
			return errors.New("firmante CMS V1 no valido o algoritmo no permitido")
		}
		cert := findLegacyCMSSignerCertificate(certs, signer.IssuerAndSerialNumber)
		if cert == nil {
			return errors.New("certificado del firmante CMS V1 no encontrado")
		}
		publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return errors.New("firmante CMS V1 no usa RSA")
		}
		signedData := plaintext
		if len(signer.AuthenticatedAttributes) > 0 {
			if err := verifyLegacyCMSAttributes(signer.AuthenticatedAttributes, plaintext); err != nil {
				return err
			}
			var err error
			signedData, err = marshalLegacyCMSAttributes(signer.AuthenticatedAttributes)
			if err != nil {
				return err
			}
		}
		digest := sha256.Sum256(signedData)
		if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signer.EncryptedDigest); err != nil {
			return errors.New("firma CMS V1 no valida")
		}
	}
	return nil
}

func findLegacyCMSSignerCertificate(certs []*x509.Certificate, issuer legacyCMSIssuerAndSerial) *x509.Certificate {
	for _, cert := range certs {
		if cert.SerialNumber.Cmp(issuer.SerialNumber) == 0 &&
			bytes.Equal(cert.RawIssuer, issuer.IssuerName.FullBytes) {
			return cert
		}
	}
	return nil
}

func verifyLegacyCMSAttributes(attributes []legacyCMSAttribute, plaintext []byte) error {
	var (
		contentTypeCount int
		digestCount      int
	)
	computedDigest := sha256.Sum256(plaintext)
	for _, attribute := range attributes {
		switch {
		case attribute.Type.Equal(oidCMSAttributeContentType):
			contentTypeCount++
			var contentType asn1.ObjectIdentifier
			rest, err := asn1.Unmarshal(attribute.Value.Bytes, &contentType)
			if err != nil || len(rest) != 0 || !contentType.Equal(oidCMSData) {
				return errors.New("atributo contentType CMS V1 no valido")
			}
		case attribute.Type.Equal(oidCMSAttributeMessageDigest):
			digestCount++
			var digest []byte
			rest, err := asn1.Unmarshal(attribute.Value.Bytes, &digest)
			if err != nil || len(rest) != 0 ||
				subtle.ConstantTimeCompare(digest, computedDigest[:]) != 1 {
				return errors.New("atributo messageDigest CMS V1 no valido")
			}
		}
	}
	if contentTypeCount != 1 || digestCount != 1 {
		return errors.New("atributos criptograficos CMS V1 incompletos o duplicados")
	}
	return nil
}

func marshalLegacyCMSAttributes(attributes []legacyCMSAttribute) ([]byte, error) {
	encoded, err := asn1.Marshal(struct {
		Attributes []legacyCMSAttribute `asn1:"set"`
	}{Attributes: attributes})
	if err != nil {
		return nil, err
	}
	var raw asn1.RawValue
	rest, err := asn1.Unmarshal(encoded, &raw)
	if err != nil || len(rest) != 0 {
		return nil, errors.New("atributos CMS V1 no validos")
	}
	return raw.Bytes, nil
}
