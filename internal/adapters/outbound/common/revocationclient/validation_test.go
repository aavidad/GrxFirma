// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"
)

type validationFixture struct {
	now   time.Time
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
	leaf  *x509.Certificate
}

func newValidationFixture(t *testing.T) validationFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Minute)
	ca, caKey := createTestCA(t, "CA de validación", big.NewInt(100), now)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber:   big.NewInt(200),
		Subject:        pkix.Name{CommonName: "Firmante"},
		NotBefore:      now.Add(-time.Hour),
		NotAfter:       now.Add(24 * time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature,
		AuthorityKeyId: append([]byte(nil), ca.SubjectKeyId...),
	}, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	return validationFixture{now: now, ca: ca, caKey: caKey, leaf: leaf}
}

func createTestCA(
	t *testing.T,
	commonName string,
	serial *big.Int,
	now time.Time,
) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	subjectKeyID := serial.FillBytes(make([]byte, 20))
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.Add(30 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		SubjectKeyId:          subjectKeyID,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func createOCSPFixture(
	t *testing.T,
	issuerForID *x509.Certificate,
	responder *x509.Certificate,
	signer crypto.Signer,
	serial *big.Int,
	status int,
	thisUpdate time.Time,
	nextUpdate time.Time,
	embedResponder bool,
) []byte {
	t.Helper()
	template := ocsp.Response{
		SerialNumber: serial,
		Status:       status,
		ThisUpdate:   thisUpdate,
		NextUpdate:   nextUpdate,
	}
	if embedResponder {
		template.Certificate = responder
	}
	if status == ocsp.Revoked {
		template.RevokedAt = thisUpdate.Add(-time.Minute)
	}
	der, err := ocsp.CreateResponse(issuerForID, responder, template, signer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func createCRLFixture(
	t *testing.T,
	issuer *x509.Certificate,
	signer crypto.Signer,
	thisUpdate time.Time,
	nextUpdate time.Time,
	revokedSerial *big.Int,
) []byte {
	t.Helper()
	template := &x509.RevocationList{
		Number:     big.NewInt(7),
		ThisUpdate: thisUpdate,
		NextUpdate: nextUpdate,
	}
	if revokedSerial != nil {
		template.RevokedCertificateEntries = []x509.RevocationListEntry{{
			SerialNumber:   revokedSerial,
			RevocationTime: thisUpdate.Add(-time.Minute),
		}}
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, issuer, signer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func newValidationClient(t *testing.T, now time.Time) *Client {
	t.Helper()
	client := newClient(&http.Client{Timeout: 2 * time.Second}, testEndpointPolicy())
	client.now = func() time.Time { return now }
	return client
}

func TestValidateOCSPResponse_ValidRevokedMismatchStaleAndBadSignature(t *testing.T) {
	fixture := newValidationFixture(t)
	otherCA, otherKey := createTestCA(t, "Otra CA", big.NewInt(300), fixture.now)
	good := createOCSPFixture(
		t, fixture.ca, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
	)
	revoked := createOCSPFixture(
		t, fixture.ca, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
		ocsp.Revoked, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
	)
	mismatchedIssuerID := createOCSPFixture(
		t, otherCA, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
	)
	stale := createOCSPFixture(
		t, fixture.ca, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-2*time.Hour), fixture.now.Add(-time.Hour), false,
	)
	badSignature := createOCSPFixture(
		t, fixture.ca, otherCA, otherKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
	)

	tests := []struct {
		name       string
		body       []byte
		wantStatus CertificateStatus
		wantErr    string
	}{
		{name: "válida", body: good, wantStatus: CertificateStatusGood},
		{name: "revocada", body: revoked, wantStatus: CertificateStatusRevoked},
		{name: "CertID de otro emisor", body: mismatchedIssuerID, wantErr: "CertID OCSP no corresponde"},
		{name: "caducada", body: stale, wantErr: "OCSP caducada"},
		{name: "firma incorrecta", body: badSignature, wantErr: "no autenticada"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, basic, err := ValidateOCSPResponse(
				tt.body,
				fixture.leaf,
				fixture.ca,
				fixture.now,
			)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v, se esperaba %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if result.Status != tt.wantStatus {
				t.Fatalf("status=%d, want %d", result.Status, tt.wantStatus)
			}
			if len(basic) == 0 {
				t.Fatal("no se devolvió el BasicOCSPResponse autenticado")
			}
		})
	}
}

func TestValidateOCSPResponse_RespondedorDelegadoRequiereOCSPSigning(t *testing.T) {
	fixture := newValidationFixture(t)
	createResponder := func(t *testing.T, usages []x509.ExtKeyUsage) (*x509.Certificate, *ecdsa.PrivateKey) {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
			SerialNumber:   big.NewInt(400),
			Subject:        pkix.Name{CommonName: "Respondedor delegado"},
			NotBefore:      fixture.now.Add(-time.Hour),
			NotAfter:       fixture.now.Add(time.Hour),
			KeyUsage:       x509.KeyUsageDigitalSignature,
			ExtKeyUsage:    usages,
			AuthorityKeyId: append([]byte(nil), fixture.ca.SubjectKeyId...),
		}, fixture.ca, &key.PublicKey, fixture.caKey)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return cert, key
	}

	authorized, authorizedKey := createResponder(t, []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning})
	validDER := createOCSPFixture(
		t, fixture.ca, authorized, authorizedKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), true,
	)
	if _, _, err := ValidateOCSPResponse(validDER, fixture.leaf, fixture.ca, fixture.now); err != nil {
		t.Fatalf("respondedor autorizado rechazado: %v", err)
	}

	unauthorized, unauthorizedKey := createResponder(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	invalidDER := createOCSPFixture(
		t, fixture.ca, unauthorized, unauthorizedKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), true,
	)
	if _, _, err := ValidateOCSPResponse(invalidDER, fixture.leaf, fixture.ca, fixture.now); err == nil ||
		!strings.Contains(err.Error(), "no autorizado") {
		t.Fatalf("respondedor sin OCSPSigning aceptado: %v", err)
	}
}

func TestValidateCRLResponse_ValidRevokedMismatchStaleAndBadSignature(t *testing.T) {
	fixture := newValidationFixture(t)
	otherCA, otherKey := createTestCA(t, "Otra CA CRL", big.NewInt(500), fixture.now)
	good := createCRLFixture(
		t, fixture.ca, fixture.caKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), nil,
	)
	revoked := createCRLFixture(
		t, fixture.ca, fixture.caKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour),
		fixture.leaf.SerialNumber,
	)
	mismatch := createCRLFixture(
		t, otherCA, otherKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), nil,
	)
	stale := createCRLFixture(
		t, fixture.ca, fixture.caKey, fixture.now.Add(-2*time.Hour), fixture.now.Add(-time.Hour), nil,
	)
	badSignature := createCRLFixture(
		t, fixture.ca, otherKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), nil,
	)

	tests := []struct {
		name       string
		body       []byte
		wantStatus CertificateStatus
		wantErr    string
	}{
		{name: "válida", body: good, wantStatus: CertificateStatusGood},
		{name: "revocada", body: revoked, wantStatus: CertificateStatusRevoked},
		{name: "emisor distinto", body: mismatch, wantErr: "emisor de la CRL"},
		{name: "caducada", body: stale, wantErr: "CRL caducada"},
		{name: "firma incorrecta", body: badSignature, wantErr: "firma de CRL no válida"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateCRLResponse(tt.body, fixture.leaf, fixture.ca, fixture.now)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v, se esperaba %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if result.Status != tt.wantStatus {
				t.Fatalf("status=%d, want %d", result.Status, tt.wantStatus)
			}
		})
	}
}

func TestClientCheck_NoOcultaEstadosTerminales(t *testing.T) {
	fixture := newValidationFixture(t)
	goodCRL := createCRLFixture(
		t, fixture.ca, fixture.caKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), nil,
	)
	for _, status := range []int{ocsp.Revoked, ocsp.Unknown} {
		t.Run(map[int]string{ocsp.Revoked: "revoked", ocsp.Unknown: "unknown"}[status], func(t *testing.T) {
			ocspDER := createOCSPFixture(
				t, fixture.ca, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
				status, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
			)
			var crlRequests atomic.Int32
			ocspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(ocspDER)
			}))
			defer ocspServer.Close()
			crlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				crlRequests.Add(1)
				_, _ = w.Write(goodCRL)
			}))
			defer crlServer.Close()
			fixture.leaf.OCSPServer = []string{ocspServer.URL}
			fixture.leaf.CRLDistributionPoints = []string{crlServer.URL}

			result, err := newValidationClient(t, fixture.now).Check(
				context.Background(),
				fixture.leaf,
				fixture.ca,
			)
			if err != nil {
				t.Fatalf("Check() error inesperado: %v", err)
			}
			want := CertificateStatusUnknown
			if status == ocsp.Revoked {
				want = CertificateStatusRevoked
			}
			if result.Status != want || result.Method != "ocsp" {
				t.Fatalf("resultado=%+v, se esperaba estado %d por OCSP", result, want)
			}
			if crlRequests.Load() != 0 {
				t.Fatal("un estado OCSP terminal activó fallback a CRL")
			}
		})
	}
}

func TestClientCheck_CRLRevocadaPrevaleceSobreOCSPGood(t *testing.T) {
	fixture := newValidationFixture(t)
	ocspDER := createOCSPFixture(
		t, fixture.ca, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
	)
	crlDER := createCRLFixture(
		t, fixture.ca, fixture.caKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour),
		fixture.leaf.SerialNumber,
	)
	ocspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(ocspDER)
	}))
	defer ocspServer.Close()
	crlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(crlDER)
	}))
	defer crlServer.Close()
	fixture.leaf.OCSPServer = []string{ocspServer.URL}
	fixture.leaf.CRLDistributionPoints = []string{crlServer.URL}

	result, err := newValidationClient(t, fixture.now).Check(context.Background(), fixture.leaf, fixture.ca)
	if err != nil {
		t.Fatalf("Check() error inesperado: %v", err)
	}
	if result.Status != CertificateStatusRevoked || result.Method != "crl" {
		t.Fatalf("resultado=%+v, se esperaba revocación por CRL", result)
	}
}

func TestClientCheck_SoloUsaFallbackCriptograficamenteValido(t *testing.T) {
	fixture := newValidationFixture(t)
	otherCA, otherKey := createTestCA(t, "Firmante OCSP incorrecto", big.NewInt(600), fixture.now)
	badOCSP := createOCSPFixture(
		t, fixture.ca, otherCA, otherKey, fixture.leaf.SerialNumber,
		ocsp.Good, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
	)
	goodCRL := createCRLFixture(
		t, fixture.ca, fixture.caKey, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), nil,
	)
	ocspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(badOCSP)
	}))
	defer ocspServer.Close()
	crlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(goodCRL)
	}))
	defer crlServer.Close()
	fixture.leaf.OCSPServer = []string{ocspServer.URL}
	fixture.leaf.CRLDistributionPoints = []string{crlServer.URL}

	result, err := newValidationClient(t, fixture.now).Check(context.Background(), fixture.leaf, fixture.ca)
	if err != nil {
		t.Fatalf("Check() error inesperado: %v", err)
	}
	if result.Status != CertificateStatusGood || result.Method != "crl" {
		t.Fatalf("resultado=%+v, se esperaba fallback CRL autenticado", result)
	}
	if len(result.Evidence.OCSPResponses) != 0 || len(result.Evidence.CRLs) != 1 {
		t.Fatalf("se devolvió evidencia no autenticada: %+v", result.Evidence)
	}
}

func TestClientCheck_RechazaEmisorAntesDeAbrirRed(t *testing.T) {
	fixture := newValidationFixture(t)
	otherCA, _ := createTestCA(t, "Emisor incorrecto", big.NewInt(700), fixture.now)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	fixture.leaf.OCSPServer = []string{server.URL}
	fixture.leaf.CRLDistributionPoints = []string{server.URL}

	_, err := newValidationClient(t, fixture.now).Check(context.Background(), fixture.leaf, otherCA)
	if err == nil || !strings.Contains(err.Error(), "emisor") {
		t.Fatalf("emisor incorrecto aceptado: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("se abrió red antes de validar la relación certificado-emisor")
	}
}

func TestClientCheck_FallaCerradoSiNoEstaConfigurado(t *testing.T) {
	fixture := newValidationFixture(t)
	for _, client := range []*Client{nil, {}} {
		result, err := client.Check(context.Background(), fixture.leaf, fixture.ca)
		if err == nil || !strings.Contains(err.Error(), "no configurado") {
			t.Fatalf("cliente=%v, error=%v", client, err)
		}
		if result.Status != CertificateStatusUnknown {
			t.Fatalf("cliente=%v, status=%d", client, result.Status)
		}
	}
}

func TestReadBoundedBody_DetectaExactamenteElPrimerByteExcedente(t *testing.T) {
	const limit = int64(8)
	got, err := readBoundedBody(bytes.NewReader([]byte("12345678")), limit, "test")
	if err != nil {
		t.Fatalf("el límite exacto fue rechazado: %v", err)
	}
	if string(got) != "12345678" {
		t.Fatalf("cuerpo=%q", got)
	}
	_, err = readBoundedBody(bytes.NewReader([]byte("123456789")), limit, "test")
	if err == nil || !strings.Contains(err.Error(), "máximo de 8 bytes") {
		t.Fatalf("el primer byte excedente no fue detectado: %v", err)
	}
}

func TestClientFetch_ExponeErroresDeEstado(t *testing.T) {
	fixture := newValidationFixture(t)
	for _, status := range []int{ocsp.Revoked, ocsp.Unknown} {
		ocspDER := createOCSPFixture(
			t, fixture.ca, fixture.ca, fixture.caKey, fixture.leaf.SerialNumber,
			status, fixture.now.Add(-time.Minute), fixture.now.Add(time.Hour), false,
		)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(ocspDER)
		}))
		fixture.leaf.OCSPServer = []string{server.URL}
		fixture.leaf.CRLDistributionPoints = nil
		_, err := newValidationClient(t, fixture.now).Fetch(context.Background(), fixture.leaf, fixture.ca)
		server.Close()
		target := ErrCertificateStatusUnknown
		if status == ocsp.Revoked {
			target = ErrCertificateRevoked
		}
		if !errors.Is(err, target) {
			t.Fatalf("status=%d, error=%v, se esperaba %v", status, err, target)
		}
	}
}
