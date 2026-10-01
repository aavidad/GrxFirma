// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//lint:file-ignore SA1019 fixture de compatibilidad; el producto nunca emite RSA PKCS#1 v1.5

package protector

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

func TestCMSProtector_LeeSignedAndEnvelopedDataV1SoloConOptIn(t *testing.T) {
	priv, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature)
	plaintext := []byte("fixture sintetico compatible con AutoFirma 1.9")
	envelope := newLegacyV1CMSTestEnvelope(t, plaintext, priv, cert)
	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	protected, err := domain.NewDocument("legacy.enveloped", envelope, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	key := domain.ProtectionKeyMaterial{
		RecipientID:               "legacy-v1",
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}

	machinepolicy.SetForTest(t, machinepolicy.PermitirCMSAESECBLegacy, false)
	disabled, err := NuevoCMSProtector().Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{key})
	if err == nil || !strings.Contains(err.Error(), machinepolicy.PermitirCMSAESECBLegacy) {
		t.Fatalf("Unprotect() sin opt-in error = %v; want instruccion explicita", err)
	}
	if len(disabled.Document.Content) != 0 {
		t.Fatal("el lector deshabilitado no debe devolver texto claro")
	}

	machinepolicy.SetForTest(t, machinepolicy.PermitirCMSAESECBLegacy, true)
	unprotected, err := newTrustedCMSTestProtector(cert).Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{key})
	if err != nil {
		t.Fatalf("Unprotect() con opt-in error = %v", err)
	}
	if !bytes.Equal(unprotected.Document.Content, plaintext) {
		t.Fatalf("contenido recuperado = %q; want %q", unprotected.Document.Content, plaintext)
	}
	if unprotected.RecipientID != key.RecipientID {
		t.Fatalf("RecipientID = %q; want %q", unprotected.RecipientID, key.RecipientID)
	}
}

func TestCMSProtector_RechazaManipulacionDeSignedAndEnvelopedDataV1(t *testing.T) {
	priv, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature)
	envelope := newLegacyV1CMSTestEnvelope(t, bytes.Repeat([]byte("contenido firmado "), 8), priv, cert)

	var info cmsContentInfo
	if _, err := asn1.Unmarshal(envelope, &info); err != nil {
		t.Fatalf("asn1.Unmarshal(ContentInfo) error = %v", err)
	}
	var legacy legacyCMSSignedEnvelopedData
	if _, err := asn1.Unmarshal(info.Content.Bytes, &legacy); err != nil {
		t.Fatalf("asn1.Unmarshal(SignedAndEnvelopedData) error = %v", err)
	}
	legacy.EncryptedContentInfo.EncryptedContent.Bytes[0] ^= 0x80
	inner, err := asn1.Marshal(legacy)
	if err != nil {
		t.Fatalf("asn1.Marshal(SignedAndEnvelopedData) error = %v", err)
	}
	info.Content.Bytes = inner
	tampered, err := asn1.Marshal(info)
	if err != nil {
		t.Fatalf("asn1.Marshal(ContentInfo) error = %v", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	protected, err := domain.NewDocument("tampered.enveloped", tampered, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	machinepolicy.SetForTest(t, machinepolicy.PermitirCMSAESECBLegacy, true)
	result, err := NuevoCMSProtector().Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{{
		RecipientID:               "legacy-v1",
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if !errors.Is(err, ErrCMSSignedEnvelopedIntegrity) {
		t.Fatalf("Unprotect(manipulado) error = %v; want rechazo de integridad", err)
	}
	if len(result.Document.Content) != 0 {
		t.Fatal("un contenedor manipulado no debe devolver texto claro")
	}
}

func TestStrictPKCS7Unpad_RechazaPaddingInvalido(t *testing.T) {
	for _, padded := range [][]byte{
		nil,
		bytes.Repeat([]byte{0}, aes.BlockSize),
		append(bytes.Repeat([]byte{1}, aes.BlockSize-2), 2, 3),
	} {
		if plaintext, err := strictPKCS7Unpad(padded, aes.BlockSize); err == nil || plaintext != nil {
			t.Fatalf("strictPKCS7Unpad(%x) = %x, %v; want rechazo", padded, plaintext, err)
		}
	}
}

func newLegacyV1CMSTestEnvelope(t *testing.T, plaintext []byte, privateKey *rsa.PrivateKey, cert *x509.Certificate) []byte {
	t.Helper()

	contentKey := bytes.Repeat([]byte{0x4b}, 16)
	// La prueba genera deliberadamente el formato exacto de AutoFirma 1.9.
	encryptedKey, err := rsa.EncryptPKCS1v15(rand.Reader, &privateKey.PublicKey, contentKey)
	if err != nil {
		t.Fatalf("EncryptPKCS1v15() error = %v", err)
	}
	padded := append([]byte(nil), plaintext...)
	paddingSize := aes.BlockSize - len(padded)%aes.BlockSize
	padded = append(padded, bytes.Repeat([]byte{byte(paddingSize)}, paddingSize)...)
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v", err)
	}
	ciphertext := make([]byte, len(padded))
	for offset := 0; offset < len(padded); offset += aes.BlockSize {
		block.Encrypt(ciphertext[offset:offset+aes.BlockSize], padded[offset:offset+aes.BlockSize])
	}

	contentTypeDER, err := asn1.Marshal(oidCMSData)
	if err != nil {
		t.Fatalf("asn1.Marshal(contentType) error = %v", err)
	}
	messageDigest := sha256.Sum256(plaintext)
	messageDigestDER, err := asn1.Marshal(messageDigest[:])
	if err != nil {
		t.Fatalf("asn1.Marshal(messageDigest) error = %v", err)
	}
	attributes := []legacyCMSAttribute{
		{
			Type: oidCMSAttributeContentType,
			Value: asn1.RawValue{
				Class:      0,
				Tag:        asn1.TagSet,
				IsCompound: true,
				Bytes:      contentTypeDER,
			},
		},
		{
			Type: oidCMSAttributeMessageDigest,
			Value: asn1.RawValue{
				Class:      0,
				Tag:        asn1.TagSet,
				IsCompound: true,
				Bytes:      messageDigestDER,
			},
		},
	}
	signedAttributes, err := marshalLegacyCMSAttributes(attributes)
	if err != nil {
		t.Fatalf("marshalLegacyCMSAttributes() error = %v", err)
	}
	attributesDigest := sha256.Sum256(signedAttributes)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, attributesDigest[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15() error = %v", err)
	}

	nullParameters := asn1.RawValue{Class: 0, Tag: asn1.TagNull}
	issuer := legacyCMSIssuerAndSerial{
		IssuerName:   asn1.RawValue{FullBytes: cert.RawIssuer},
		SerialNumber: cert.SerialNumber,
	}
	legacy := legacyCMSSignedEnvelopedData{
		Version: 1,
		RecipientInfos: []legacyCMSRecipientInfo{{
			Version:               0,
			IssuerAndSerialNumber: issuer,
			KeyEncryptionAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidCMSRSAEncryption,
				Parameters: nullParameters,
			},
			EncryptedKey: encryptedKey,
		}},
		DigestAlgorithmIdentifiers: []pkix.AlgorithmIdentifier{{
			Algorithm:  oidCMSSHA256,
			Parameters: nullParameters,
		}},
		EncryptedContentInfo: legacyCMSEncryptedContentInfo{
			// AutoFirma 1.9 usaba aquí id-encryptedData, aunque el atributo
			// contentType firmado siguiera declarando id-data.
			ContentType: oidCMSEncryptedData,
			ContentEncryptionAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidCMSLegacyAESGeneric,
				Parameters: nullParameters,
			},
			EncryptedContent: asn1.RawValue{
				Class:      2,
				Tag:        0,
				IsCompound: false,
				Bytes:      ciphertext,
			},
		},
		Certificates: asn1.RawValue{
			Class:      2,
			Tag:        1,
			IsCompound: true,
			Bytes:      cert.Raw,
		},
		SignerInfos: []legacyCMSSignerInfo{{
			Version:               1,
			IssuerAndSerialNumber: issuer,
			DigestAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidCMSSHA256,
				Parameters: nullParameters,
			},
			AuthenticatedAttributes: attributes,
			DigestEncryptionAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidCMSRSAEncryption,
				Parameters: nullParameters,
			},
			EncryptedDigest: signature,
		}},
	}
	inner, err := asn1.Marshal(legacy)
	if err != nil {
		t.Fatalf("asn1.Marshal(SignedAndEnvelopedData) error = %v", err)
	}
	envelope, err := asn1.Marshal(cmsContentInfo{
		ContentType: oidCMSSignedEnvelopedData,
		Content: asn1.RawValue{
			Class:      2,
			Tag:        0,
			IsCompound: true,
			Bytes:      inner,
		},
	})
	if err != nil {
		t.Fatalf("asn1.Marshal(ContentInfo) error = %v", err)
	}
	return envelope
}
