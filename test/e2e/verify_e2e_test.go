// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

type trustAnchorVacio struct{}

func (t trustAnchorVacio) Anchors(context.Context) (domain.CertificateChain, error) {
	return domain.CertificateChain{}, nil
}

func generarCredencialesE2E(t *testing.T, cn string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar la clave RSA de prueba: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("no se pudo crear el certificado de prueba: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("no se pudo parsear el certificado de prueba: %v", err)
	}
	return priv, cert
}

func TestVerifyE2E_PAdES(t *testing.T) {
	priv, cert := generarCredencialesE2E(t, "E2E-PAdES")
	motor := desktopsigner.NuevoMotorFirmaGo(nil)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)

	doc, _ := domain.NewDocument("demo.pdf", pdffixture.Minimal(), "application/pdf")
	signed, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
	}, clave)
	if err != nil {
		t.Fatalf("no se pudo firmar el documento PAdES: %v", err)
	}

	uc := application.NuevoVerifySignatureUseCase(trustAnchorVacio{}, commonsigner.NewMultiVerifier(), nil)
	signedDoc, _ := domain.NewDocument("demo.pdf", signed.Data, "application/pdf")
	result, err := uc.Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: signedDoc})
	if err != nil {
		t.Fatalf("la verificación E2E PAdES falló: %v", err)
	}
	if !result.Verification.Valid {
		t.Fatalf("la verificación E2E PAdES debería ser válida: %s", result.Verification.Reason)
	}
	if len(result.Firmantes) != 1 {
		t.Fatalf("se esperaba un firmante en PAdES, obtenidos %d", len(result.Firmantes))
	}
}

func TestVerifyE2E_CAdESDetachedConOriginal(t *testing.T) {
	priv, cert := generarCredencialesE2E(t, "E2E-CAdES-Detached")
	motor := desktopsigner.NuevoMotorFirmaGo(nil)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)

	original, _ := domain.NewDocument("demo.txt", []byte("contenido detached e2e"), "text/plain")
	signed, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: original,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}, clave)
	if err != nil {
		t.Fatalf("no se pudo firmar el documento CAdES detached: %v", err)
	}

	uc := application.NuevoVerifySignatureUseCase(trustAnchorVacio{}, commonsigner.NewMultiVerifier(), nil)
	signedDoc, _ := domain.NewDocument("demo.csig", signed.Data, "application/pkcs7-signature")
	result, err := uc.Ejecutar(context.Background(), application.VerifyCommand{
		SignedDocument:   signedDoc,
		OriginalDocument: &original,
	})
	if err != nil {
		t.Fatalf("la verificación E2E CAdES detached falló: %v", err)
	}
	if !result.Verification.Valid {
		t.Fatalf("la verificación E2E CAdES detached debería ser válida: %s", result.Verification.Reason)
	}
	if len(result.Firmantes) != 1 {
		t.Fatalf("se esperaba un firmante en CAdES detached, obtenidos %d", len(result.Firmantes))
	}
}
