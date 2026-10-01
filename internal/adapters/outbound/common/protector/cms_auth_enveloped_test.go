// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"grxfirma/internal/domain"
)

func TestCMSAuthEnvelopedData_NoDistingueAlteracionDelTagYFalloOAEP(t *testing.T) {
	privateKey, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	recipient := newCMSRecipient(t, "auth-tag", cert)
	doc, err := domainDocumentForCMSAuthTest("tag.bin", []byte("contenido autenticado"))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	protected, err := NuevoCMSProtector().Protect(context.Background(), doc, []domain.ProtectionRecipient{recipient})
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}

	var info cmsContentInfo
	if _, err := asn1.Unmarshal(protected.Document.Content, &info); err != nil {
		t.Fatalf("asn1.Unmarshal(ContentInfo) error = %v", err)
	}
	var envelope cmsAuthEnvelopedData
	if _, err := asn1.Unmarshal(info.Content.Bytes, &envelope); err != nil {
		t.Fatalf("asn1.Unmarshal(AuthEnvelopedData) error = %v", err)
	}
	envelope.MAC[0] ^= 0xff
	altered := marshalCMSAuthEnvelopedTest(t, envelope)
	alteredDoc, err := domain.NewDocument("tag.authenveloped.p7m", altered, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument(altered) error = %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	_, err = NuevoCMSProtector().Unprotect(context.Background(), alteredDoc, []domain.ProtectionKeyMaterial{{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if !errors.Is(err, errCMSAuthEnvelopedDecrypt) {
		t.Fatalf("Unprotect(tag alterado) error = %v; want %v", err, errCMSAuthEnvelopedDecrypt)
	}
	if errors.Is(err, errCMSAuthEnvelopedIntegrity) {
		t.Fatalf("Unprotect(tag alterado) expuso la causa criptografica: %v", err)
	}
	tagError := err.Error()

	envelope.MAC[0] ^= 0xff
	envelope.RecipientInfos[0].EncryptedKey[0] ^= 0xff
	altered = marshalCMSAuthEnvelopedTest(t, envelope)
	alteredDoc, err = domain.NewDocument("oaep.authenveloped.p7m", altered, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument(oaep alterado) error = %v", err)
	}
	_, err = NuevoCMSProtector().Unprotect(context.Background(), alteredDoc, []domain.ProtectionKeyMaterial{{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if !errors.Is(err, errCMSAuthEnvelopedDecrypt) || err.Error() != tagError {
		t.Fatalf("Unprotect(OAEP alterado) error = %v; want error indistinguible %q", err, tagError)
	}
}

func TestCMSAuthEnvelopedData_RechazaCamposDERSobrantes(t *testing.T) {
	_, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	recipient := newCMSRecipient(t, "auth-der", cert)
	job, err := domainDocumentForCMSAuthTest("der.bin", []byte("contenido"))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	protected, err := NuevoCMSProtector().Protect(context.Background(), job, []domain.ProtectionRecipient{recipient})
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}

	var info cmsContentInfo
	if _, err := asn1.Unmarshal(protected.Document.Content, &info); err != nil {
		t.Fatalf("asn1.Unmarshal(ContentInfo) error = %v", err)
	}
	var original cmsAuthEnvelopedData
	if _, err := asn1.Unmarshal(info.Content.Bytes, &original); err != nil {
		t.Fatalf("asn1.Unmarshal(AuthEnvelopedData) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(cmsAuthEnvelopedData) []byte
	}{
		{
			name: "parametros GCM",
			mutate: func(envelope cmsAuthEnvelopedData) []byte {
				parameters := envelope.AuthEncryptedContentInfo.ContentEncryptionAlgorithm.Parameters.FullBytes
				envelope.AuthEncryptedContentInfo.ContentEncryptionAlgorithm.Parameters = asn1.RawValue{
					FullBytes: appendCMSAuthEnvelopedSequenceElement(t, parameters, []byte{0x05, 0x00}),
				}
				return marshalCMSAuthEnvelopedTest(t, envelope)
			},
		},
		{
			name: "parametros OAEP",
			mutate: func(envelope cmsAuthEnvelopedData) []byte {
				parameters := envelope.RecipientInfos[0].KeyEncryptionAlgorithm.Parameters.FullBytes
				envelope.RecipientInfos[0].KeyEncryptionAlgorithm.Parameters = asn1.RawValue{
					FullBytes: appendCMSAuthEnvelopedSequenceElement(t, parameters, []byte{0x05, 0x00}),
				}
				return marshalCMSAuthEnvelopedTest(t, envelope)
			},
		},
		{
			name: "estructura principal",
			mutate: func(envelope cmsAuthEnvelopedData) []byte {
				inner, err := asn1.Marshal(envelope)
				if err != nil {
					t.Fatalf("asn1.Marshal(AuthEnvelopedData) error = %v", err)
				}
				inner = appendCMSAuthEnvelopedSequenceElement(t, inner, []byte{0x05, 0x00})
				wrapper, err := asn1.Marshal(cmsContentInfo{
					ContentType: oidCMSAuthEnvelopedData,
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
				return wrapper
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCMSAuthEnvelopedData(tt.mutate(original))
			if err == nil {
				t.Fatalf("parseCMSAuthEnvelopedData(DER con sobrantes) error = %v", err)
			}
		})
	}
}

func TestCMSAuthEnvelopedData_InteropOpenSSL(t *testing.T) {
	opensslPath, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL no disponible")
	}

	privateKey, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	recipient := newCMSRecipient(t, "auth-openssl", cert)
	plaintext := []byte("AuthEnvelopedData interoperable\x00binario")
	job, err := domainDocumentForCMSAuthTest("auth.bin", plaintext)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "recipient-cert.pem")
	keyPath := filepath.Join(tmp, "recipient-key.pem")
	plainPath := filepath.Join(tmp, "plain.bin")
	goEnvelopePath := filepath.Join(tmp, "go-auth-enveloped.der")
	goPlainPath := filepath.Join(tmp, "go-plain.bin")
	opensslEnvelopePath := filepath.Join(tmp, "openssl-auth-enveloped.der")
	opensslLegacyEnvelopePath := filepath.Join(tmp, "openssl-auth-enveloped-pkcs1.der")
	writeCMSTestFile(t, certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600)
	writeCMSTestFile(t, keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600)
	writeCMSTestFile(t, plainPath, plaintext, 0o600)

	goEnvelope, err := NuevoCMSProtector().Protect(context.Background(), job, []domain.ProtectionRecipient{recipient})
	if err != nil {
		t.Fatalf("Protect(AuthEnvelopedData) error = %v", err)
	}
	writeCMSTestFile(t, goEnvelopePath, goEnvelope.Document.Content, 0o600)
	runOpenSSL(t, opensslPath,
		"cms", "-cmsout", "-noout", "-inform", "DER",
		"-in", goEnvelopePath,
	)
	runOpenSSL(t, opensslPath,
		"cms", "-decrypt", "-binary", "-inform", "DER",
		"-in", goEnvelopePath,
		"-recip", certPath,
		"-inkey", keyPath,
		"-out", goPlainPath,
	)
	goPlain, err := os.ReadFile(goPlainPath)
	if err != nil {
		t.Fatalf("ReadFile(OpenSSL plaintext) error = %v", err)
	}
	if !bytes.Equal(goPlain, plaintext) {
		t.Fatalf("OpenSSL recupero %q; want %q", goPlain, plaintext)
	}

	runOpenSSL(t, opensslPath,
		"cms", "-encrypt", "-binary",
		"-in", plainPath,
		"-outform", "DER",
		"-out", opensslEnvelopePath,
		"-aes-256-gcm",
		"-recip", certPath,
		"-keyopt", "rsa_padding_mode:oaep",
		"-keyopt", "rsa_oaep_md:sha256",
		"-keyopt", "rsa_mgf1_md:sha256",
	)
	opensslEnvelope, err := os.ReadFile(opensslEnvelopePath)
	if err != nil {
		t.Fatalf("ReadFile(OpenSSL AuthEnvelopedData) error = %v", err)
	}
	protected, err := domain.NewDocument("openssl.authenveloped.p7m", opensslEnvelope, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument(OpenSSL AuthEnvelopedData) error = %v", err)
	}
	unprotected, err := NuevoCMSProtector().Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if err != nil {
		t.Fatalf("Unprotect(OpenSSL AuthEnvelopedData) error = %v", err)
	}
	if !bytes.Equal(unprotected.Document.Content, plaintext) {
		t.Fatalf("Go recupero %q; want %q", unprotected.Document.Content, plaintext)
	}

	runOpenSSL(t, opensslPath,
		"cms", "-encrypt", "-binary",
		"-in", plainPath,
		"-outform", "DER",
		"-out", opensslLegacyEnvelopePath,
		"-aes-256-gcm",
		certPath,
	)
	opensslLegacyEnvelope, err := os.ReadFile(opensslLegacyEnvelopePath)
	if err != nil {
		t.Fatalf("ReadFile(OpenSSL AuthEnvelopedData PKCS#1) error = %v", err)
	}
	legacyProtected, err := domain.NewDocument("openssl-pkcs1.authenveloped.p7m", opensslLegacyEnvelope, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument(OpenSSL AuthEnvelopedData PKCS#1) error = %v", err)
	}
	_, err = NuevoCMSProtector().Unprotect(context.Background(), legacyProtected, []domain.ProtectionKeyMaterial{{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("RSA-OAEP-SHA-256")) {
		t.Fatalf("Unprotect(OpenSSL AuthEnvelopedData PKCS#1) error = %v; want rechazo de transporte legacy", err)
	}
}

func domainDocumentForCMSAuthTest(name string, content []byte) (domain.ProtectionJob, error) {
	doc, err := domain.NewDocument(name, content, "application/octet-stream")
	if err != nil {
		return domain.ProtectionJob{}, err
	}
	return domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{protectionContainerOptionKey: "auth-enveloped-data"},
	}, nil
}

func marshalCMSAuthEnvelopedTest(t *testing.T, envelope cmsAuthEnvelopedData) []byte {
	t.Helper()
	inner, err := asn1.Marshal(envelope)
	if err != nil {
		t.Fatalf("asn1.Marshal(AuthEnvelopedData) error = %v", err)
	}
	wrapper, err := asn1.Marshal(cmsContentInfo{
		ContentType: oidCMSAuthEnvelopedData,
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
	return wrapper
}

func appendCMSAuthEnvelopedSequenceElement(t *testing.T, der, element []byte) []byte {
	t.Helper()
	var sequence asn1.RawValue
	rest, err := asn1.Unmarshal(der, &sequence)
	if err != nil || len(rest) != 0 || sequence.Class != 0 ||
		sequence.Tag != asn1.TagSequence || !sequence.IsCompound {
		t.Fatalf("secuencia DER de prueba no valida: rest=%d err=%v", len(rest), err)
	}
	sequence.FullBytes = nil
	sequence.Bytes = append(append([]byte(nil), sequence.Bytes...), element...)
	altered, err := asn1.Marshal(sequence)
	if err != nil {
		t.Fatalf("asn1.Marshal(secuencia alterada) error = %v", err)
	}
	return altered
}
