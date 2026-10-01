// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/domain"
)

func TestEnvelopeProtector_CompatRSAOAEP(t *testing.T) {
	engine := protector.NuevoEnvelopeProtector()
	doc, err := domain.NewDocument("secreto.txt", []byte("contenido confidencial"), "text/plain")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-1",
		Label:                  "Compat RSA",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         mustSelfSignedCertDER(t, priv),
	}})
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if protected.Profile != domain.ProtectionProfileCompat {
		t.Fatalf("perfil inesperado: %s", protected.Profile)
	}

	unprotected, err := engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:               "dest-1",
		RSAOAEP256PrivateKeyPKCS8: privDER,
		CertificateDER:            mustSelfSignedCertDER(t, priv),
	}})
	if err != nil {
		t.Fatalf("Unprotect() error = %v", err)
	}
	if got := string(unprotected.Document.Content); got != "contenido confidencial" {
		t.Fatalf("contenido inesperado: %q", got)
	}
}

func TestEnvelopeProtector_StrongMLKEMX25519(t *testing.T) {
	engine := protector.NuevoEnvelopeProtector()
	doc, err := domain.NewDocument("alto.bin", []byte("secreto muy serio"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	mlkemPriv, err := mlkem.GenerateKey768()
	if err != nil {
		t.Fatalf("mlkem.GenerateKey768() error = %v", err)
	}
	xPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ecdh.GenerateKey() error = %v", err)
	}

	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileStrong,
	}, []domain.ProtectionRecipient{{
		ID:                "dest-pq",
		Label:             "ML-KEM+X25519",
		MLKEM768PublicKey: mlkemPriv.EncapsulationKey().Bytes(),
		X25519PublicKey:   xPriv.PublicKey().Bytes(),
	}})
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}

	unprotected, err := engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:      "dest-pq",
		MLKEM768Seed:     mlkemPriv.Bytes(),
		X25519PrivateKey: xPriv.Bytes(),
	}})
	if err != nil {
		t.Fatalf("Unprotect() error = %v", err)
	}
	if got := string(unprotected.Document.Content); got != "secreto muy serio" {
		t.Fatalf("contenido inesperado: %q", got)
	}
}

func TestEnvelopeProtector_UnprotectWrongRecipientFails(t *testing.T) {
	engine := protector.NuevoEnvelopeProtector()
	doc, err := domain.NewDocument("secreto.txt", []byte("contenido"), "text/plain")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	otherPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	otherPrivDER, err := x509.MarshalPKCS8PrivateKey(otherPriv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-ok",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         mustSelfSignedCertDER(t, priv),
	}})
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}

	if _, err := engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:               "dest-ok",
		RSAOAEP256PrivateKeyPKCS8: otherPrivDER,
		CertificateDER:            mustSelfSignedCertDER(t, otherPriv),
	}}); err == nil {
		t.Fatal("se esperaba error al usar una clave privada incorrecta")
	}
}

func TestAdaptiveProtector_CompatCMS(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.pdf", []byte("cms compatible"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	certDER := mustSelfSignedCertDER(t, priv)

	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{"container": "cms"},
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-cms",
		Label:                  "Compat CMS",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         certDER,
	}})
	if err != nil {
		t.Fatalf("Protect(CMS) error = %v", err)
	}
	if got := protected.Document.Name; got != "secreto.pdf.enveloped" {
		t.Fatalf("nombre protegido inesperado: %q", got)
	}
	if got := protected.Document.MIMEType; got != domain.MIMETypeProtectedCMS {
		t.Fatalf("MIME protegido inesperado: %q", got)
	}

	unprotected, err := engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:               "dest-cms",
		RSAOAEP256PrivateKeyPKCS8: privDER,
		CertificateDER:            certDER,
	}})
	if err != nil {
		t.Fatalf("Unprotect(CMS) error = %v", err)
	}
	if got := string(unprotected.Document.Content); got != "cms compatible" {
		t.Fatalf("contenido inesperado: %q", got)
	}
	if got := unprotected.Document.Name; got != "secreto.pdf" {
		t.Fatalf("nombre desprotegido inesperado: %q", got)
	}
}

func TestAdaptiveProtector_CompatCMSEncryptedData(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.pdf", []byte("cms encrypted"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	secret := []byte("0123456789abcdef0123456789abcdef")

	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document:     doc,
		Profile:      domain.ProtectionProfileCompat,
		SymmetricKey: secret,
		Options: map[string]string{
			"container": "cms-encrypted",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Protect(CMS encrypted) error = %v", err)
	}
	if got := protected.Document.Name; got != "secreto.pdf.encrypted.p7m" {
		t.Fatalf("nombre protegido inesperado: %q", got)
	}
	if got := protected.Document.MIMEType; got != domain.MIMETypeProtectedCMS {
		t.Fatalf("MIME protegido inesperado: %q", got)
	}
	if protected.RecipientCount != 0 {
		t.Fatalf("recipientCount inesperado: %d", protected.RecipientCount)
	}

	unprotected, err := engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:  "psk-1",
		SymmetricKey: secret,
	}})
	if err != nil {
		t.Fatalf("Unprotect(CMS encrypted) error = %v", err)
	}
	if got := string(unprotected.Document.Content); got != "cms encrypted" {
		t.Fatalf("contenido inesperado: %q", got)
	}
	if got := unprotected.RecipientID; got != "psk-1" {
		t.Fatalf("recipientID inesperado: %q", got)
	}
}

func TestAdaptiveProtector_ProtegeCMSConcurrentemente(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	secret := []byte("0123456789abcdef0123456789abcdef")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	certDER := mustSelfSignedCertDER(t, priv)
	recipient := domain.ProtectionRecipient{
		ID:                     "dest-concurrente",
		Label:                  "Compat CMS concurrente",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         certDER,
	}

	const operations = 24
	errs := make(chan error, operations)
	for i := 0; i < operations; i++ {
		i := i
		go func() {
			content := []byte(fmt.Sprintf("contenido-%d", i))
			doc, err := domain.NewDocument(fmt.Sprintf("secreto-%d.bin", i), content, "application/octet-stream")
			if err != nil {
				errs <- err
				return
			}

			job := domain.ProtectionJob{
				Document: doc,
				Profile:  domain.ProtectionProfileCompat,
			}
			var (
				recipients []domain.ProtectionRecipient
				keys       []domain.ProtectionKeyMaterial
			)
			if i%2 == 0 {
				job.Options = map[string]string{
					"container":  "cms-encrypted-data",
					"secret_b64": base64.StdEncoding.EncodeToString(secret),
				}
				keys = []domain.ProtectionKeyMaterial{{
					RecipientID:  "secret-concurrente",
					SymmetricKey: secret,
				}}
			} else {
				job.Options = map[string]string{"container": "cms"}
				recipients = []domain.ProtectionRecipient{recipient}
				keys = []domain.ProtectionKeyMaterial{{
					RecipientID:               recipient.ID,
					RSAOAEP256PrivateKeyPKCS8: privDER,
					CertificateDER:            certDER,
				}}
			}

			protected, err := engine.Protect(context.Background(), job, recipients)
			if err != nil {
				errs <- fmt.Errorf("operacion %d, Protect(): %w", i, err)
				return
			}
			unprotected, err := engine.Unprotect(context.Background(), protected.Document, keys)
			if err != nil {
				errs <- fmt.Errorf("operacion %d, Unprotect(): %w", i, err)
				return
			}
			if string(unprotected.Document.Content) != string(content) {
				errs <- fmt.Errorf("operacion %d: contenido inesperado %q", i, unprotected.Document.Content)
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < operations; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestAdaptiveProtector_RechazaSecretoCMSConLongitudInvalida(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("cms encrypted"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	secret := []byte("0123456789abcdef0123456789abcdef")
	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options: map[string]string{
			"container":  "cms-encrypted",
			"secret_b64": base64.StdEncoding.EncodeToString(secret),
		},
	}, nil)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}

	_, err = engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:  "secret-invalido",
		SymmetricKey: make([]byte, 31),
	}})
	if err == nil || !strings.Contains(err.Error(), "exactamente 32 bytes") {
		t.Fatalf("Unprotect() error = %v", err)
	}
}

func TestAdaptiveProtector_RejectsUnknownProtectionContainer(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("hola"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	if _, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{"container": "cms-raro"},
	}, nil); err == nil || !strings.Contains(err.Error(), "contenedor de proteccion no soportado") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RejectsUnsupportedCMSContentTypeOnProtect(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("hola"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	certDER := mustSelfSignedCertDER(t, priv)
	_, err = engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{"container": "signedandenvelopeddata"},
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-1",
		Label:                  "Compat RSA",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         certDER,
	}})
	if err == nil || !strings.Contains(err.Error(), "SignedAndEnvelopedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RoundTripAuthEnvelopedCMS(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("hola"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	certDER := mustSelfSignedCertDER(t, priv)
	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{"container": "authenvelopeddata"},
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-1",
		Label:                  "Compat RSA",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         certDER,
	}})
	if err != nil {
		t.Fatalf("Protect(AuthEnvelopedData) error = %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	unprotected, err := engine.Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID:               "dest-1",
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            certDER,
	}})
	if err != nil {
		t.Fatalf("Unprotect(AuthEnvelopedData) error = %v", err)
	}
	if !bytes.Equal(unprotected.Document.Content, doc.Content) {
		t.Fatalf("contenido recuperado = %q; want %q", unprotected.Document.Content, doc.Content)
	}
}

func TestAdaptiveProtector_RejectsAuthenticatedDataCMSOnProtect(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("hola"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	certDER := mustSelfSignedCertDER(t, priv)
	_, err = engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{"container": "authenticateddata"},
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-1",
		Label:                  "Compat RSA",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         certDER,
	}})
	if err == nil || !strings.Contains(err.Error(), "AuthenticatedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RejectsEncryptedDataCMSWithClearError(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("cms encrypted"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	protected, err := engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options: map[string]string{
			"container":  "cms-encrypted",
			"secret_b64": base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		},
	}, nil)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if _, err := engine.Unprotect(context.Background(), protected.Document, nil); err == nil || !strings.Contains(err.Error(), "EncryptedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RejectsAuthEnvelopedCMSWithClearError(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	protectedRaw := mustCMSContentInfo(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 23})
	protectedDoc, err := domain.NewDocument("secreto-authenv.p7m", protectedRaw, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	if _, err := engine.Unprotect(context.Background(), protectedDoc, nil); err == nil || !strings.Contains(err.Error(), "AuthEnvelopedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RejectsAuthenticatedDataCMSWithClearError(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	protectedRaw := mustCMSContentInfo(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 2})
	protectedDoc, err := domain.NewDocument("secreto-authenticated.p7m", protectedRaw, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	if _, err := engine.Unprotect(context.Background(), protectedDoc, nil); err == nil || !strings.Contains(err.Error(), "AuthenticatedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RejectsCompressedDataCMSOnProtect(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	doc, err := domain.NewDocument("secreto.bin", []byte("hola"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	certDER := mustSelfSignedCertDER(t, priv)
	_, err = engine.Protect(context.Background(), domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options:  map[string]string{"container": "compresseddata"},
	}, []domain.ProtectionRecipient{{
		ID:                     "dest-1",
		Label:                  "Compat RSA",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         certDER,
	}})
	if err == nil || !strings.Contains(err.Error(), "CompressedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func TestAdaptiveProtector_RejectsCompressedDataCMSWithClearError(t *testing.T) {
	engine := protector.NuevoAdaptiveProtector()
	protectedRaw := mustCMSContentInfo(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 9})
	protectedDoc, err := domain.NewDocument("secreto-compressed.p7m", protectedRaw, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	if _, err := engine.Unprotect(context.Background(), protectedDoc, nil); err == nil || !strings.Contains(err.Error(), "CompressedData") {
		t.Fatalf("error inesperado = %v", err)
	}
}

func mustSelfSignedCertDER(t *testing.T, priv *rsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Compat Test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	return der
}

type cmsContentInfoTest struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

func mustCMSContentInfo(t *testing.T, oid asn1.ObjectIdentifier) []byte {
	t.Helper()
	raw, err := asn1.Marshal(cmsContentInfoTest{
		ContentType: oid,
	})
	if err != nil {
		t.Fatalf("asn1.Marshal() error = %v", err)
	}
	return raw
}
