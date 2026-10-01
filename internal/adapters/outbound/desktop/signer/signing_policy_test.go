// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/signingpolicy"
)

type signingPolicyClock struct{ now time.Time }

func (c signingPolicyClock) Now() time.Time { return c.now }

func signingPolicyCertificate(t *testing.T, key crypto.Signer, now time.Time) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestMotorFirmaGo_PreflightAntesDeTodoFormatoYAccion(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := signingPolicyCertificate(t, private, now.Add(-2*time.Hour))
	// Estos metadatos manipulados no sustituyen las fechas firmadas del DER.
	cert.NotAfter = now.Add(time.Hour)
	key := NuevaClaveLocal(private, cert)
	motor := NuevoMotorFirmaGo(signingPolicyClock{now})
	for _, format := range []domain.SignatureFormat{domain.FormatCAdES, domain.FormatPAdES, domain.FormatXAdES, "XMLdSig", "FacturaE", "ASiC-XAdES", "ODF", "OOXML"} {
		for _, action := range []domain.SignatureAction{domain.ActionSign, domain.ActionCoSign, domain.ActionCounterSign} {
			t.Run(string(format)+"/"+string(action), func(t *testing.T) {
				job := domain.SignatureJob{Document: domain.Document{Content: []byte("documento sintético")}, Format: format, Action: action,
					Options: map[string]string{"skipCertificateValidation": "true", "allowExpired": "true", "signingTime": now.Add(-2 * time.Hour).Format(time.RFC3339)}}
				result, err := motor.Sign(context.Background(), job, key)
				var typed *signingpolicy.Error
				if !errors.As(err, &typed) || typed.Reason != signingpolicy.Expired || len(result.Data) != 0 {
					t.Fatalf("preflight no impidió la firma: result=%+v error=%v", result, err)
				}
			})
		}
	}
}

func TestMotorFirmaGo_PreflightRelojInyectadoYNil(t *testing.T) {
	now := time.Date(2020, 1, 2, 12, 0, 0, 0, time.UTC)
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := NuevaClaveLocal(private, signingPolicyCertificate(t, private, now))
	job := domain.SignatureJob{Document: domain.Document{Content: []byte("firma histórica sintética")}, Format: domain.FormatCAdES, Action: domain.ActionSign}
	result, err := NuevoMotorFirmaGo(signingPolicyClock{now}).Sign(context.Background(), job, key)
	if err != nil || len(result.Data) == 0 {
		t.Fatalf("firma en fecha válida: %v", err)
	}
	_, err = NuevoMotorFirmaGo(signingPolicyClock{now.Add(2 * time.Hour)}).Sign(context.Background(), job, key)
	if !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) {
		t.Fatalf("firma fuera de vigencia: %v", err)
	}
	var missing *ClaveLocal
	_, err = NuevoMotorFirmaGo(nil).Sign(context.Background(), job, missing)
	if !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) {
		t.Fatalf("clave nil: %v", err)
	}
}

func TestClaveLocal_PreflightTrifasico(t *testing.T) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := NuevaClaveLocal(private, signingPolicyCertificate(t, private, time.Now().Add(-2*time.Hour)))
	if result, err := key.SignDigest(make([]byte, crypto.SHA256.Size()), crypto.SHA256); !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) || len(result) != 0 {
		t.Fatalf("SignDigest: resultado=%x error=%v", result, err)
	}
	if result, err := key.SignPreData([]byte("PRE"), "SHA256withECDSA", map[string]string{"allowExpired": "true"}); !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) || result != "" {
		t.Fatalf("SignPreData: resultado=%s error=%v", result, err)
	}
}
