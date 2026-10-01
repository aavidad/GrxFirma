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
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
)

func TestCMSProtector_SignedAndEnvelopedDistingueIntegridadVigenciaYConfianza(t *testing.T) {
	rootKey, root := newCMSRootCA(t, "Root confiable")
	signerKey, signer := newCMSSignedLeaf(t, rootKey, root, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	_, otherRoot := newCMSRootCA(t, "Root ajena")
	protected, key, plaintext := newCMSTrustEnvelope(t, signerKey, signer, root)

	t.Run("cadena_confiable", func(t *testing.T) {
		result, err := newTrustedCMSTestProtector(root).Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{key})
		if err != nil {
			t.Fatalf("Unprotect() error = %v", err)
		}
		if !bytes.Equal(result.Document.Content, plaintext) {
			t.Fatalf("contenido = %q; want %q", result.Document.Content, plaintext)
		}
	})

	t.Run("cadena_no_confiable", func(t *testing.T) {
		result, err := newTrustedCMSTestProtector(otherRoot).Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{key})
		if !errors.Is(err, ErrCMSSignedEnvelopedTrust) {
			t.Fatalf("Unprotect() error = %v; want ErrCMSSignedEnvelopedTrust", err)
		}
		if len(result.Document.Content) != 0 {
			t.Fatal("una cadena no confiable no debe devolver texto claro")
		}
	})

	t.Run("sin_proveedor_de_anclas", func(t *testing.T) {
		result, err := NuevoCMSProtectorConAnclas(nil).Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{key})
		if !errors.Is(err, ErrCMSSignedEnvelopedTrust) {
			t.Fatalf("Unprotect() error = %v; want ErrCMSSignedEnvelopedTrust", err)
		}
		if len(result.Document.Content) != 0 {
			t.Fatal("la ausencia de anclas no debe devolver texto claro")
		}
	})

	t.Run("certificado_caducado", func(t *testing.T) {
		expiredKey, expired := newCMSSignedLeaf(t, rootKey, root, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
		expiredProtected, expiredRecipient, _ := newCMSTrustEnvelope(t, expiredKey, expired, root, cmsSigningPolicyClock{expired.NotBefore.Add(time.Minute)})
		result, err := newTrustedCMSTestProtector(root).Unprotect(
			context.Background(),
			expiredProtected,
			[]domain.ProtectionKeyMaterial{expiredRecipient},
		)
		if !errors.Is(err, ErrCMSSignedEnvelopedValidity) {
			t.Fatalf("Unprotect() error = %v; want ErrCMSSignedEnvelopedValidity", err)
		}
		if len(result.Document.Content) != 0 {
			t.Fatal("un firmante caducado no debe devolver texto claro")
		}
	})

	t.Run("certificado_sin_uso_de_firma", func(t *testing.T) {
		_, invalid := newCMSSignedLeafWithUsage(
			t,
			rootKey,
			root,
			time.Now().Add(-time.Hour),
			time.Now().Add(time.Hour),
			x509.KeyUsageKeyEncipherment,
		)
		err := validateCMSSignedEnvelopedSignerValidity([]*x509.Certificate{invalid}, time.Now())
		if !errors.Is(err, ErrCMSSignedEnvelopedValidity) {
			t.Fatalf("validateCMSSignedEnvelopedSignerValidity() error = %v; want ErrCMSSignedEnvelopedValidity", err)
		}
	})
}

func TestCMSProtector_SignedAndEnvelopedRespetaLimiteYCancelacion(t *testing.T) {
	tooLarge, err := domain.NewDocument(
		"demasiado-grande.p7m",
		bytes.Repeat([]byte{0x30}, maxCMSSignedEnvelopedBytes+1),
		domain.MIMETypeProtectedCMS,
	)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	result, err := decryptSignedEnvelopedCMS(context.Background(), tooLarge, nil, cmsTestTrustAnchors{})
	if err == nil || !strings.Contains(err.Error(), "tamano") {
		t.Fatalf("decryptSignedEnvelopedCMS() error = %v; want limite", err)
	}
	if len(result.Document.Content) != 0 {
		t.Fatal("un contenedor fuera de limite no debe devolver contenido")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = decryptSignedEnvelopedCMS(ctx, domain.Document{}, nil, cmsTestTrustAnchors{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("decryptSignedEnvelopedCMS(cancelado) error = %v", err)
	}
	if len(result.Document.Content) != 0 {
		t.Fatal("una operación cancelada no debe devolver contenido")
	}
}

func TestCMSProtector_LegacyV1ExigeConfianzaAdemasDeOptIn(t *testing.T) {
	privateKey, signer := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature)
	_, otherRoot := newCMSRootCA(t, "Root ajena V1")
	envelope := newLegacyV1CMSTestEnvelope(t, []byte("migracion V1"), privateKey, signer)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	protected, err := domain.NewDocument("legacy.enveloped", envelope, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	machinepolicy.SetForTest(t, machinepolicy.PermitirCMSAESECBLegacy, true)
	result, err := newTrustedCMSTestProtector(otherRoot).Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{{
		RecipientID:               "legacy-v1",
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            signer.Raw,
	}})
	if !errors.Is(err, ErrCMSSignedEnvelopedTrust) {
		t.Fatalf("Unprotect(V1) error = %v; want ErrCMSSignedEnvelopedTrust", err)
	}
	if len(result.Document.Content) != 0 {
		t.Fatal("un sobre V1 de firmante no confiable no debe devolver texto claro")
	}
}

func newCMSTrustEnvelope(t *testing.T, signerKey *rsa.PrivateKey, signer, root *x509.Certificate, clocks ...ports.Clock) (domain.Document, domain.ProtectionKeyMaterial, []byte) {
	t.Helper()
	plaintext := []byte("contenido SignedAndEnvelopedData con confianza explícita")
	document, err := domain.NewDocument("trust.txt", plaintext, "text/plain")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	recipient := newCMSRecipient(t, "trust-recipient", signer)
	engine := NuevoCMSSignedEnvelopedProtector()
	if len(clocks) > 0 {
		engine.WithClock(clocks[0])
	}
	protected, err := engine.ProtectAndSign(
		context.Background(),
		domain.ProtectionJob{
			Document: document,
			Profile:  domain.ProtectionProfileCompat,
			Options:  map[string]string{protectionContainerOptionKey: "signedandenvelopeddata"},
		},
		[]domain.ProtectionRecipient{recipient},
		&commonsigner.LocalSigningKey{
			ID:          "trust-signer",
			Signer:      signerKey,
			Certificate: signer,
			Chain:       []*x509.Certificate{root},
		},
	)
	if err != nil {
		t.Fatalf("ProtectAndSign() error = %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(signerKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	return protected.Document, domain.ProtectionKeyMaterial{
		RecipientID:               recipient.ID,
		RSAOAEP256PrivateKeyPKCS8: privateDER,
		CertificateDER:            signer.Raw,
	}, plaintext
}

func newCMSRootCA(t *testing.T, commonName string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate(root) error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(root) error = %v", err)
	}
	return privateKey, cert
}

func newCMSSignedLeaf(t *testing.T, rootKey *rsa.PrivateKey, root *x509.Certificate, notBefore, notAfter time.Time) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	return newCMSSignedLeafWithUsage(
		t,
		rootKey,
		root,
		notBefore,
		notAfter,
		x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
	)
}

func newCMSSignedLeafWithUsage(t *testing.T, rootKey *rsa.PrivateKey, root *x509.Certificate, notBefore, notAfter time.Time, keyUsage x509.KeyUsage) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "Firmante CMS"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     keyUsage,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, root, &privateKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("CreateCertificate(leaf) error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(leaf) error = %v", err)
	}
	return privateKey, cert
}
