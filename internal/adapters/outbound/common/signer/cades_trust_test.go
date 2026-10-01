// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

type cadesTrustFixture struct {
	root         *x509.Certificate
	intermediate *x509.Certificate
	leaf         *x509.Certificate
	leafKey      *ecdsa.PrivateKey
}

func TestCAdESVerifier_TrustChain(t *testing.T) {
	fixture := newCAdESTrustFixture(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), x509.KeyUsageDigitalSignature)
	content := []byte("contenido firmado para probar confianza")
	cms := signCAdESTrustFixture(t, fixture, content, false)
	verifier := NewCAdESVerifier()

	t.Run("raiz_confiable_y_cadena_intermedia", func(t *testing.T) {
		result, _, err := verifier.VerifyDetachedCMSWithAnchors(context.Background(), cms, content, domain.CertificateChain{
			DERCertificates: [][]byte{fixture.root.Raw},
		})
		if err != nil {
			t.Fatalf("VerifyDetachedCMSWithAnchors() error = %v", err)
		}
		if !result.Valid {
			t.Fatalf("Valid=false con cadena confiable: %s (%v)", result.Reason, result.Trust.Details)
		}
		if result.Integrity.Status != domain.VerificationStatusValid {
			t.Fatalf("Integrity.Status=%q, want valid", result.Integrity.Status)
		}
		if result.Trust.Status != domain.VerificationStatusValid {
			t.Fatalf("Trust.Status=%q, want valid: %v", result.Trust.Status, result.Trust.Details)
		}
		if !containsStringFragment(result.Trust.Details, "chain_length=3") {
			t.Fatalf("no se verificó la cadena raíz-intermedia-hoja: %v", result.Trust.Details)
		}
	})

	t.Run("combina_raices_del_sistema_y_anclas_explicitas", func(t *testing.T) {
		result, _, err := verifier.VerifyDetachedCMSWithAnchors(context.Background(), cms, content, domain.CertificateChain{
			DERCertificates: [][]byte{fixture.root.Raw},
			UseSystemRoots:  true,
		})
		if err != nil {
			t.Fatalf("VerifyDetachedCMSWithAnchors() error = %v", err)
		}
		if !result.Valid || result.Trust.Status != domain.VerificationStatusValid {
			t.Fatalf("la ancla explícita debe combinarse con el pool del sistema: valid=%v trust=%+v", result.Valid, result.Trust)
		}
	})

	t.Run("raiz_no_confiable", func(t *testing.T) {
		other := newCAdESTrustFixture(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), x509.KeyUsageDigitalSignature)
		result, _, err := verifier.VerifyDetachedCMSWithAnchors(context.Background(), cms, content, domain.CertificateChain{
			DERCertificates: [][]byte{other.root.Raw},
		})
		if err != nil {
			t.Fatalf("VerifyDetachedCMSWithAnchors() error = %v", err)
		}
		if result.Valid {
			t.Fatal("una cadena no anclada no puede producir Valid=true")
		}
		if result.Integrity.Status != domain.VerificationStatusValid {
			t.Fatalf("la firma matemática debe seguir siendo válida, Integrity.Status=%q", result.Integrity.Status)
		}
		if result.Trust.Status != domain.VerificationStatusInvalid {
			t.Fatalf("Trust.Status=%q, want invalid", result.Trust.Status)
		}
		if len(result.Errors) == 0 {
			t.Fatal("la cadena no confiable debe quedar reflejada en Errors")
		}
	})

	t.Run("sin_anclas_no_inventa_confianza", func(t *testing.T) {
		result, _, err := verifier.VerifyDetachedCMS(context.Background(), cms, content)
		if err != nil {
			t.Fatalf("VerifyDetachedCMS() error = %v", err)
		}
		if !result.Valid {
			t.Fatalf("la integridad criptográfica debe conservar compatibilidad sin anclas: %s", result.Reason)
		}
		if result.Integrity.Status != domain.VerificationStatusValid {
			t.Fatalf("Integrity.Status=%q, want valid", result.Integrity.Status)
		}
		if result.Trust.Status != domain.VerificationStatusUnknown {
			t.Fatalf("Trust.Status=%q, want unknown", result.Trust.Status)
		}
		if !containsStringFragment(result.Warnings, "sin anclas de confianza") {
			t.Fatalf("falta aviso explícito de confianza no evaluada: %v", result.Warnings)
		}
	})
}

func TestCAdESVerifier_AnclaLegacyRequiereHuellaExacta(t *testing.T) {
	fixture := newCAdESTrustFixture(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), x509.KeyUsageDigitalSignature)
	content := []byte("ancla legacy")
	cms := signCAdESTrustFixture(t, fixture, content, true)
	verifier := NewCAdESVerifier()

	sum := sha256.Sum256(fixture.root.Raw)
	result, _, err := verifier.VerifyDetachedCMSWithAnchors(context.Background(), cms, content, domain.CertificateChain{
		Certificates: []domain.CertificateRef{{
			Subject:     fixture.root.Subject.String(),
			Fingerprint: strings.ToUpper(hex.EncodeToString(sum[:])),
		}},
	})
	if err != nil {
		t.Fatalf("VerifyDetachedCMSWithAnchors() error = %v", err)
	}
	if !result.Valid || result.Trust.Status != domain.VerificationStatusValid {
		t.Fatalf("la huella exacta del ancla embebida debe validar: valid=%v trust=%+v", result.Valid, result.Trust)
	}

	result, _, err = verifier.VerifyDetachedCMSWithAnchors(context.Background(), cms, content, domain.CertificateChain{
		Certificates: []domain.CertificateRef{{Subject: fixture.root.Subject.String()}},
	})
	if err != nil {
		t.Fatalf("VerifyDetachedCMSWithAnchors() error = %v", err)
	}
	if result.Valid || result.Trust.Status != domain.VerificationStatusInvalid {
		t.Fatalf("el Subject por sí solo no puede anclar confianza: valid=%v trust=%+v", result.Valid, result.Trust)
	}
}

func TestCAdESVerifier_CertificadoInvalidoNoContaminaIntegridad(t *testing.T) {
	tests := []struct {
		name     string
		notAfter time.Time
		keyUsage x509.KeyUsage
		reason   string
	}{
		{
			name:     "caducado",
			notAfter: time.Now().Add(-time.Minute),
			keyUsage: x509.KeyUsageDigitalSignature,
			reason:   "caducado",
		},
		{
			name:     "sin_uso_de_firma",
			notAfter: time.Now().Add(time.Hour),
			keyUsage: x509.KeyUsageKeyEncipherment,
			reason:   "no autorizado para firma digital",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			notBefore := time.Now().Add(-2 * time.Hour)
			fixture := newCAdESTrustFixture(t, notBefore, tc.notAfter, tc.keyUsage)
			content := []byte("certificado inválido con firma matemática correcta")
			cms := signCAdESTrustFixture(t, fixture, content, false)

			result, _, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), cms, content)
			if err != nil {
				t.Fatalf("VerifyDetachedCMS() error = %v", err)
			}
			if result.Valid {
				t.Fatal("un certificado firmante inválido no puede producir Valid=true")
			}
			if result.Integrity.Status != domain.VerificationStatusValid {
				t.Fatalf("Integrity.Status=%q, want valid", result.Integrity.Status)
			}
			if result.Certificate.Status != domain.VerificationStatusInvalid {
				t.Fatalf("Certificate.Status=%q, want invalid", result.Certificate.Status)
			}
			if !strings.Contains(result.Certificate.Reason, tc.reason) {
				t.Fatalf("Certificate.Reason=%q, falta %q", result.Certificate.Reason, tc.reason)
			}
		})
	}
}

func newCAdESTrustFixture(t *testing.T, leafNotBefore, leafNotAfter time.Time, leafKeyUsage x509.KeyUsage) cadesTrustFixture {
	t.Helper()

	rootKey := newECDSAKey(t)
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1001),
		Subject:               pkix.Name{CommonName: "Raíz GrxFirma"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	root := createCertificate(t, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)

	intermediateKey := newECDSAKey(t)
	intermediateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1002),
		Subject:               pkix.Name{CommonName: "Intermedia GrxFirma"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(12 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	intermediate := createCertificate(t, intermediateTemplate, root, &intermediateKey.PublicKey, rootKey)

	leafKey := newECDSAKey(t)
	leafTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1003),
		Subject:               pkix.Name{CommonName: "Firmante GrxFirma"},
		NotBefore:             leafNotBefore,
		NotAfter:              leafNotAfter,
		BasicConstraintsValid: true,
		KeyUsage:              leafKeyUsage,
	}
	leaf := createCertificate(t, leafTemplate, intermediate, &leafKey.PublicKey, intermediateKey)

	return cadesTrustFixture{
		root:         root,
		intermediate: intermediate,
		leaf:         leaf,
		leafKey:      leafKey,
	}
}

func newECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	return key
}

func createCertificate(t *testing.T, template, parent *x509.Certificate, publicKey any, signerKey *ecdsa.PrivateKey) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, signerKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return cert
}

func signCAdESTrustFixture(t *testing.T, fixture cadesTrustFixture, content []byte, embedRoot bool) []byte {
	t.Helper()
	chain := []*x509.Certificate{fixture.intermediate}
	if embedRoot {
		chain = append(chain, fixture.root)
	}
	cms, _, err := signDetachedCAdESBES(content, &LocalSigningKey{
		ID:          "trust-test",
		Signer:      fixture.leafKey,
		Certificate: fixture.leaf,
		Chain:       chain,
	}, cmsHashSpecs[0])
	if err != nil {
		t.Fatalf("signDetachedCAdESBES() error = %v", err)
	}
	return cms
}

func containsStringFragment(items []string, fragment string) bool {
	for _, item := range items {
		if strings.Contains(item, fragment) {
			return true
		}
	}
	return false
}
