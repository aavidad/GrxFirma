// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

var (
	oidTestRSAEncryption = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidTestSHA256WithRSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidTestSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidTestAES256GCM     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 46}
	oidTestAES256CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidTestRSAESOAEP     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 7}
)

func TestCMSProtector_EmiteAlgoritmosDERReales(t *testing.T) {
	priv, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature)
	recipient := newCMSRecipient(t, "dest-der", cert)
	doc, err := domain.NewDocument("oid.bin", []byte("algoritmos cms"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	enveloped, err := NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{protectionContainerOptionKey: protectionContainerCMS},
	}, []domain.ProtectionRecipient{recipient})
	if err != nil {
		t.Fatalf("Protect(EnvelopedData) error = %v", err)
	}
	envelopedAlgorithms := inspectCMSAlgorithms(t, enveloped.Document.Content)
	requireCMSOID(t, "EnvelopedData content type", envelopedAlgorithms.contentType, oidCMSEnvelopedData)
	requireCMSOID(t, "EnvelopedData content cipher", envelopedAlgorithms.contentEncryption, oidTestAES256GCM)
	requireCMSOID(t, "EnvelopedData key transport", envelopedAlgorithms.keyEncryption[0], oidTestRSAEncryption)
	if envelopedAlgorithms.keyEncryption[0].Equal(oidTestRSAESOAEP) {
		t.Fatal("EnvelopedData declara RSA-OAEP, pero la dependencia cifra con PKCS#1 v1.5")
	}

	authEnveloped, err := NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{protectionContainerOptionKey: "auth-enveloped-data"},
	}, []domain.ProtectionRecipient{recipient})
	if err != nil {
		t.Fatalf("Protect(AuthEnvelopedData) error = %v", err)
	}
	authEnvelopedAlgorithms := inspectCMSAlgorithms(t, authEnveloped.Document.Content)
	requireCMSOID(t, "AuthEnvelopedData content type", authEnvelopedAlgorithms.contentType, oidCMSAuthEnvelopedData)
	requireCMSOID(t, "AuthEnvelopedData content cipher", authEnvelopedAlgorithms.contentEncryption, oidTestAES256GCM)
	requireCMSOID(t, "AuthEnvelopedData key transport", authEnvelopedAlgorithms.keyEncryption[0], oidTestRSAESOAEP)

	secret := bytes.Repeat([]byte{0x5a}, protectionAES256KeyBytes)
	encrypted, err := NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options: map[string]string{
			protectionContainerOptionKey: "cms-encrypted",
			protectionSecretOptionKey:    base64.StdEncoding.EncodeToString(secret),
		},
	}, nil)
	if err != nil {
		t.Fatalf("Protect(EncryptedData) error = %v", err)
	}
	encryptedAlgorithms := inspectCMSAlgorithms(t, encrypted.Document.Content)
	requireCMSOID(t, "EncryptedData content type", encryptedAlgorithms.contentType, oidCMSEncryptedData)
	requireCMSOID(t, "EncryptedData content cipher", encryptedAlgorithms.contentEncryption, oidTestAES256GCM)
	if len(encryptedAlgorithms.keyEncryption) != 0 {
		t.Fatalf("EncryptedData no debe contener transporte de clave: %v", encryptedAlgorithms.keyEncryption)
	}

	signed, err := NuevoCMSSignedEnvelopedProtector().ProtectAndSign(
		context.Background(),
		domain.ProtectionJob{
			Document: doc,
			Profile:  domain.ProtectionProfileCompat,
			Options:  map[string]string{protectionContainerOptionKey: "signedandenvelopeddata"},
		},
		[]domain.ProtectionRecipient{recipient},
		&commonsigner.LocalSigningKey{
			ID:          "signer-der",
			Signer:      priv,
			Certificate: cert,
		},
	)
	if err != nil {
		t.Fatalf("ProtectAndSign() error = %v", err)
	}
	signedAlgorithms := inspectCMSAlgorithms(t, signed.Document.Content)
	requireCMSOID(t, "SignedAndEnvelopedData content type", signedAlgorithms.contentType, oidCMSSignedEnvelopedData)
	requireCMSOID(t, "SignedAndEnvelopedData content cipher", signedAlgorithms.contentEncryption, oidTestAES256GCM)
	requireCMSOID(t, "SignedAndEnvelopedData key transport", signedAlgorithms.keyEncryption[0], oidTestRSAEncryption)
	requireCMSOID(t, "SignedAndEnvelopedData digest", signedAlgorithms.digest[0], oidTestSHA256)
	requireCMSOID(t, "SignedAndEnvelopedData signature", signedAlgorithms.signature[0], oidTestSHA256WithRSA)

	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	unprotected, err := newTrustedCMSTestProtector(cert).Unprotect(context.Background(), signed.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if err != nil {
		t.Fatalf("Unprotect(SignedAndEnvelopedData) error = %v", err)
	}
	if !bytes.Equal(unprotected.Document.Content, doc.Content) {
		t.Fatalf("contenido recuperado = %q; want %q", unprotected.Document.Content, doc.Content)
	}
}

func TestCMSProtector_RechazaClavePublicaDeclaradaDistintaDelCertificado(t *testing.T) {
	_, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	otherPrivate, _ := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	otherPublicDER, err := x509.MarshalPKIXPublicKey(&otherPrivate.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	doc, err := domain.NewDocument("mismatch.bin", []byte("contenido"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	_, err = NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{protectionContainerOptionKey: protectionContainerCMS},
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-mismatch",
		RSAOAEP256PublicKeyDER: otherPublicDER,
		CertificateDER:         cert.Raw,
	}})
	if err == nil || !strings.Contains(err.Error(), "no coincide") {
		t.Fatalf("Protect() error = %v; want rechazo por clave/certificado distintos", err)
	}
}

func TestCMSProtector_RechazaRSAConSoloKeyAgreement(t *testing.T) {
	_, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyAgreement)
	doc, err := domain.NewDocument("key-usage.bin", []byte("contenido"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	_, err = NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{protectionContainerOptionKey: protectionContainerCMS},
	}, []domain.ProtectionRecipient{newCMSRecipient(t, "dest-key-agreement", cert)})
	if err == nil || !strings.Contains(err.Error(), "no permite cifrado de clave") {
		t.Fatalf("Protect() error = %v; want rechazo por KeyUsage", err)
	}
}

func TestCMSSignedEnvelopedProtector_RechazaFirmanteDistintoDelCertificado(t *testing.T) {
	_, signerCert := newCMSRSAIdentity(t, x509.KeyUsageDigitalSignature)
	otherPrivate, _ := newCMSRSAIdentity(t, x509.KeyUsageDigitalSignature)
	_, recipientCert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	doc, err := domain.NewDocument("mismatch.bin", []byte("contenido"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	_, err = NuevoCMSSignedEnvelopedProtector().ProtectAndSign(
		context.Background(),
		domain.ProtectionJob{Document: doc, Profile: domain.ProtectionProfileCompat},
		[]domain.ProtectionRecipient{newCMSRecipient(t, "recipient", recipientCert)},
		&commonsigner.LocalSigningKey{
			ID:          "signer-mismatch",
			Signer:      otherPrivate,
			Certificate: signerCert,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no corresponde") {
		t.Fatalf("ProtectAndSign() error = %v; want rechazo por clave/certificado distintos", err)
	}
}

func TestCMSProtector_InteropEnvelopedDataConOpenSSL(t *testing.T) {
	opensslPath, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL no disponible")
	}

	priv, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	recipient := newCMSRecipient(t, "dest-openssl", cert)
	plaintext := []byte("interoperabilidad CMS con OpenSSL\x00binaria")
	doc, err := domain.NewDocument("interop.bin", plaintext, "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "recipient-cert.pem")
	plainPath := filepath.Join(tmp, "plain.bin")
	goEnvelopePath := filepath.Join(tmp, "go-envelope.der")
	opensslEnvelopePath := filepath.Join(tmp, "openssl-envelope.der")
	writeCMSTestFile(t, certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600)
	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	writeCMSTestFile(t, plainPath, plaintext, 0o600)

	goEnvelope, err := NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{protectionContainerOptionKey: protectionContainerCMS},
	}, []domain.ProtectionRecipient{recipient})
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	writeCMSTestFile(t, goEnvelopePath, goEnvelope.Document.Content, 0o600)
	runOpenSSL(t, opensslPath,
		"cms", "-cmsout", "-noout", "-inform", "DER",
		"-in", goEnvelopePath,
	)

	runOpenSSL(t, opensslPath,
		"cms", "-encrypt", "-binary",
		"-in", plainPath,
		"-outform", "DER",
		"-out", opensslEnvelopePath,
		"-aes-256-cbc",
		certPath,
	)
	opensslEnvelope, err := os.ReadFile(opensslEnvelopePath)
	if err != nil {
		t.Fatalf("ReadFile(OpenSSL envelope) error = %v", err)
	}
	opensslAlgorithms := inspectCMSAlgorithms(t, opensslEnvelope)
	requireCMSOID(t, "OpenSSL EnvelopedData content cipher", opensslAlgorithms.contentEncryption, oidTestAES256CBC)
	protected, err := domain.NewDocument("openssl.enveloped", opensslEnvelope, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument(OpenSSL envelope) error = %v", err)
	}
	unprotected, err := NuevoCMSProtector().Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if err != nil {
		t.Fatalf("Unprotect(OpenSSL EnvelopedData) error = %v", err)
	}
	if !bytes.Equal(unprotected.Document.Content, plaintext) {
		t.Fatalf("Go recupero %q; want %q", unprotected.Document.Content, plaintext)
	}
}

func TestCMSProtector_LeeEnvelopedDataV2ConParametrosGCMLegacy(t *testing.T) {
	priv, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	plaintext := []byte("contenedor V2 previo a la correccion DER")
	secret := bytes.Repeat([]byte{0x6b}, protectionAES256KeyBytes)

	originalAlgorithm := pkcs7.ContentEncryptionAlgorithm
	pkcs7.ContentEncryptionAlgorithm = pkcs7.EncryptionAlgorithmAES256GCM
	legacyEnvelope, err := pkcs7.Encrypt(plaintext, []*x509.Certificate{cert})
	if err != nil {
		pkcs7.ContentEncryptionAlgorithm = originalAlgorithm
		t.Fatalf("pkcs7.Encrypt() error = %v", err)
	}
	legacyEncrypted, err := pkcs7.EncryptUsingPSK(plaintext, secret)
	pkcs7.ContentEncryptionAlgorithm = originalAlgorithm
	if err != nil {
		t.Fatalf("pkcs7.EncryptUsingPSK() error = %v", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	protected, err := domain.NewDocument("legacy-v2.enveloped", legacyEnvelope, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	unprotected, err := NuevoCMSProtector().Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{{
		RecipientID:               "legacy-v2",
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            cert.Raw,
	}})
	if err != nil {
		t.Fatalf("Unprotect(legacy V2) error = %v", err)
	}
	if !bytes.Equal(unprotected.Document.Content, plaintext) {
		t.Fatalf("contenido recuperado = %q; want %q", unprotected.Document.Content, plaintext)
	}

	protectedEncrypted, err := domain.NewDocument("legacy-v2.encrypted.p7m", legacyEncrypted, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument(EncryptedData) error = %v", err)
	}
	unprotectedEncrypted, err := NuevoCMSProtector().Unprotect(context.Background(), protectedEncrypted, []domain.ProtectionKeyMaterial{{
		RecipientID:  "legacy-v2-psk",
		SymmetricKey: secret,
	}})
	if err != nil {
		t.Fatalf("Unprotect(EncryptedData legacy V2) error = %v", err)
	}
	if !bytes.Equal(unprotectedEncrypted.Document.Content, plaintext) {
		t.Fatalf("contenido EncryptedData recuperado = %q; want %q", unprotectedEncrypted.Document.Content, plaintext)
	}
}

type cmsTestAlgorithms struct {
	contentType       asn1.ObjectIdentifier
	contentEncryption asn1.ObjectIdentifier
	keyEncryption     []asn1.ObjectIdentifier
	digest            []asn1.ObjectIdentifier
	signature         []asn1.ObjectIdentifier
}

func inspectCMSAlgorithms(t *testing.T, der []byte) cmsTestAlgorithms {
	t.Helper()
	var info cmsContentInfo
	rest, err := asn1.Unmarshal(der, &info)
	if err != nil {
		t.Fatalf("asn1.Unmarshal(ContentInfo) error = %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("ContentInfo contiene %d bytes DER sobrantes", len(rest))
	}
	inner := cmsTestSequenceElements(t, info.Content.Bytes)
	out := cmsTestAlgorithms{contentType: info.ContentType}

	var (
		recipientsRaw  asn1.RawValue
		contentInfoRaw asn1.RawValue
		digestRaw      asn1.RawValue
		signersRaw     asn1.RawValue
	)
	switch {
	case info.ContentType.Equal(oidCMSEnvelopedData):
		if len(inner) != 3 {
			t.Fatalf("EnvelopedData tiene %d campos; want 3", len(inner))
		}
		recipientsRaw = inner[1]
		contentInfoRaw = inner[2]
	case info.ContentType.Equal(oidCMSEncryptedData):
		if len(inner) != 2 {
			t.Fatalf("EncryptedData tiene %d campos; want 2", len(inner))
		}
		contentInfoRaw = inner[1]
	case info.ContentType.Equal(oidCMSSignedEnvelopedData):
		if len(inner) < 5 {
			t.Fatalf("SignedAndEnvelopedData tiene %d campos; want al menos 5", len(inner))
		}
		recipientsRaw = inner[1]
		digestRaw = inner[2]
		contentInfoRaw = inner[3]
		signersRaw = inner[len(inner)-1]
	case info.ContentType.Equal(oidCMSAuthEnvelopedData):
		if len(inner) != 4 {
			t.Fatalf("AuthEnvelopedData tiene %d campos; want 4", len(inner))
		}
		recipientsRaw = inner[1]
		contentInfoRaw = inner[2]
	default:
		t.Fatalf("ContentInfo no inspeccionable: %v", info.ContentType)
	}

	if len(recipientsRaw.FullBytes) != 0 {
		for _, recipientRaw := range cmsTestCollectionElements(t, recipientsRaw) {
			fields := cmsTestSequenceElements(t, recipientRaw.FullBytes)
			if len(fields) < 4 {
				t.Fatalf("RecipientInfo tiene %d campos; want al menos 4", len(fields))
			}
			out.keyEncryption = append(out.keyEncryption, cmsTestAlgorithmOID(t, fields[2]))
		}
	}
	contentFields := cmsTestSequenceElements(t, contentInfoRaw.FullBytes)
	if len(contentFields) < 2 {
		t.Fatalf("EncryptedContentInfo tiene %d campos; want al menos 2", len(contentFields))
	}
	out.contentEncryption = cmsTestAlgorithmOID(t, contentFields[1])

	if len(digestRaw.FullBytes) != 0 {
		for _, algorithmRaw := range cmsTestCollectionElements(t, digestRaw) {
			out.digest = append(out.digest, cmsTestAlgorithmOID(t, algorithmRaw))
		}
	}
	if len(signersRaw.FullBytes) != 0 {
		for _, signerRaw := range cmsTestCollectionElements(t, signersRaw) {
			fields := cmsTestSequenceElements(t, signerRaw.FullBytes)
			if len(fields) < 5 {
				t.Fatalf("SignerInfo tiene %d campos; want al menos 5", len(fields))
			}
			out.signature = append(out.signature, cmsTestAlgorithmOID(t, fields[len(fields)-2]))
		}
	}
	return out
}

func cmsTestAlgorithmOID(t *testing.T, raw asn1.RawValue) asn1.ObjectIdentifier {
	t.Helper()
	var algorithm pkix.AlgorithmIdentifier
	rest, err := asn1.Unmarshal(raw.FullBytes, &algorithm)
	if err != nil {
		t.Fatalf("asn1.Unmarshal(AlgorithmIdentifier) error = %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("AlgorithmIdentifier contiene %d bytes sobrantes", len(rest))
	}
	return algorithm.Algorithm
}

func cmsTestSequenceElements(t *testing.T, der []byte) []asn1.RawValue {
	t.Helper()
	var sequence asn1.RawValue
	rest, err := asn1.Unmarshal(der, &sequence)
	if err != nil {
		t.Fatalf("asn1.Unmarshal(SEQUENCE) error = %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("SEQUENCE contiene %d bytes sobrantes", len(rest))
	}
	if sequence.Class != 0 || sequence.Tag != asn1.TagSequence || !sequence.IsCompound {
		t.Fatalf("DER no es SEQUENCE: class=%d tag=%d compound=%t", sequence.Class, sequence.Tag, sequence.IsCompound)
	}
	return cmsTestRawElements(t, sequence.Bytes)
}

func cmsTestCollectionElements(t *testing.T, collection asn1.RawValue) []asn1.RawValue {
	t.Helper()
	if collection.Class != 0 || (collection.Tag != asn1.TagSet && collection.Tag != asn1.TagSequence) || !collection.IsCompound {
		t.Fatalf("DER no es SET/SEQUENCE: class=%d tag=%d compound=%t", collection.Class, collection.Tag, collection.IsCompound)
	}
	return cmsTestRawElements(t, collection.Bytes)
}

func cmsTestRawElements(t *testing.T, der []byte) []asn1.RawValue {
	t.Helper()
	var out []asn1.RawValue
	for len(der) > 0 {
		var raw asn1.RawValue
		rest, err := asn1.Unmarshal(der, &raw)
		if err != nil {
			t.Fatalf("asn1.Unmarshal(elemento) error = %v", err)
		}
		if len(rest) >= len(der) {
			t.Fatal("el parser ASN.1 no avanzo")
		}
		out = append(out, raw)
		der = rest
	}
	return out
}

func requireCMSOID(t *testing.T, label string, got, want asn1.ObjectIdentifier) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("%s = %v; want %v", label, got, want)
	}
}

type cmsTestTrustAnchors struct {
	chain domain.CertificateChain
	err   error
}

func (p cmsTestTrustAnchors) Anchors(ctx context.Context) (domain.CertificateChain, error) {
	if err := ctx.Err(); err != nil {
		return domain.CertificateChain{}, err
	}
	return p.chain, p.err
}

func newTrustedCMSTestProtector(certs ...*x509.Certificate) *CMSProtector {
	anchors := make([][]byte, 0, len(certs))
	for _, cert := range certs {
		if cert != nil {
			anchors = append(anchors, append([]byte(nil), cert.Raw...))
		}
	}
	return NuevoCMSProtectorConAnclas(cmsTestTrustAnchors{
		chain: domain.CertificateChain{DERCertificates: anchors},
	})
}

func newCMSRSAIdentity(t *testing.T, usage x509.KeyUsage) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "CMS test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     usage,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return priv, cert
}

func newCMSRecipient(t *testing.T, id string, cert *x509.Certificate) domain.ProtectionRecipient {
	t.Helper()
	publicDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey() error = %v", err)
	}
	return domain.ProtectionRecipient{
		ID:                     id,
		RSAOAEP256PublicKeyDER: publicDER,
		CertificateDER:         cert.Raw,
	}
}

func writeCMSTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", filepath.Base(path), err)
	}
}

func runOpenSSL(t *testing.T, opensslPath string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, opensslPath, args...).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("OpenSSL excedio el tiempo limite: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("OpenSSL %s error = %v\n%s", strings.Join(args, " "), err, output)
	}
}
