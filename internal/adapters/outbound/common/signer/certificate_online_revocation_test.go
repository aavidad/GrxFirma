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
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestCheckCertificateOnlineRevocation_SinCadena(t *testing.T) {
	result, err := CheckCertificateOnlineRevocation(context.Background(), nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if result.Status != CertificateOnlineRevocationUnavailable {
		t.Fatalf("status = %s, want %s", result.Status, CertificateOnlineRevocationUnavailable)
	}
	if result.UserMessage == "" {
		t.Fatal("UserMessage no puede quedar vacío")
	}
}

func TestCheckCertificateOnlineRevocation_SinEmisor(t *testing.T) {
	leafDER := []byte{
		0x30, 0x82, 0x01, 0x00,
	}
	_, _, err := parseCertificateOnlineRevocationChain([][]byte{leafDER})
	if err == nil {
		t.Fatal("se esperaba error al parsear DER inválido")
	}
}

func TestCheckCertificateOnlineRevocation_SinAIAExplicaCausa(t *testing.T) {
	_, ca, key := createOnlineRevocationTestCA(t)
	leaf := createOnlineRevocationTestLeaf(t, ca, key)
	result, err := CheckCertificateOnlineRevocation(context.Background(), [][]byte{leaf})
	if err != nil || result.Status != CertificateOnlineRevocationUnavailable {
		t.Fatalf("resultado = %+v, %v", result, err)
	}
	if !strings.Contains(result.Reason, "AIA") || !strings.Contains(result.UserMessage, "emisor") {
		t.Fatalf("no explica el emisor ausente: %+v", result)
	}
}

func TestResolveOnlineCertificateChain_DescargaEmisorAusente(t *testing.T) {
	_, ca, key := createOnlineRevocationTestCA(t)
	leaf := createOnlineRevocationTestLeaf(t, ca, key)
	called := 0
	gotLeaf, issuer, err := resolveOnlineCertificateChain(context.Background(), [][]byte{leaf}, func(_ context.Context, cert *x509.Certificate) (*x509.Certificate, error) {
		called++
		if cert == nil || cert.CheckSignatureFrom(ca) != nil {
			t.Fatal("AIA recibió otra hoja")
		}
		return ca, nil
	})
	if err != nil || called != 1 || gotLeaf == nil || !issuer.Equal(ca) {
		t.Fatalf("cadena = %v, %v, %v, %d", gotLeaf, issuer, err, called)
	}
}

func TestFirstCertificateURL(t *testing.T) {
	got := firstCertificateURL([]string{" ", "", "http://ocsp.example.test"})
	if got != "http://ocsp.example.test" {
		t.Fatalf("got %q", got)
	}
}

func TestCertificateOnlineRevocationResultZeroTimesAreSafe(t *testing.T) {
	result := CertificateOnlineRevocationResult{}
	if !result.CheckedAt.IsZero() || !result.RevokedAt.IsZero() {
		t.Fatal("los tiempos cero no deben mutarse")
	}
	_ = time.Second
}

func TestCheckCertificateOnlineRevocation_SinServiciosRevocacion(t *testing.T) {
	t.Parallel()

	caDER, caCert, caKey := createOnlineRevocationTestCA(t)
	leafDER := createOnlineRevocationTestLeaf(t, caCert, caKey)

	result, err := CheckCertificateOnlineRevocation(context.Background(), [][]byte{leafDER, caDER})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if result.Status != CertificateOnlineRevocationInconclusive {
		t.Fatalf("status = %s, want %s", result.Status, CertificateOnlineRevocationInconclusive)
	}
	if result.UserMessage == "" {
		t.Fatal("UserMessage no puede quedar vacío")
	}
	if result.Method != "" {
		t.Fatalf("method = %q, want vacío", result.Method)
	}
}

func createOnlineRevocationTestCA(t *testing.T) ([]byte, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey CA: %v", err)
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA Revocation Test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate CA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate CA: %v", err)
	}
	return der, cert, key
}

func createOnlineRevocationTestLeaf(t *testing.T, issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey leaf: %v", err)
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Leaf Revocation Test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("CreateCertificate leaf: %v", err)
	}
	return der
}
