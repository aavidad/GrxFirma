// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/signer"
)

// generarCertChain crea una cadena de 2 certificados: CA raíz → hoja.
func generarCertChain(t *testing.T) (leaf, ca *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave CA: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA Test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("crear cert CA: %v", err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	leafKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave hoja: %v", err)
	}
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "Hoja Test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, caCert, &leafKey2.PublicKey, caKey)
	if err != nil {
		t.Fatalf("crear cert hoja: %v", err)
	}
	leafCert, _ := x509.ParseCertificate(leafDER)
	return leafCert, caCert, caKey
}

func clienteProxyRevocacion(t *testing.T, proxy *httptest.Server, timeout time.Duration) *http.Client {
	t.Helper()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	return &http.Client{Transport: transport, Timeout: timeout}
}

// TestCAdESVerifier_CadenaVacia retorna inválido con cadena vacía.
func TestCAdESVerifier_CadenaVacia(t *testing.T) {
	t.Parallel()
	v := signer.NewCAdESVerifier()
	res, err := v.VerifyRevocation(context.Background(), nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if res.Valid {
		t.Error("cadena vacía debería ser inválida")
	}
}

// TestCAdESVerifier_SinOCSPNiCRL usa un checker que siempre falla (sin OCSP/CRL en el cert).
// El resultado debe ser válido con estado "desconocido" (degraded gracefully).
func TestCAdESVerifier_SinOCSPNiCRL(t *testing.T) {
	t.Parallel()
	leaf, ca, _ := generarCertChain(t)
	// Los certificados de test no tienen OCSP ni CRL, RevocationChecker retorna Unknown.
	v := signer.NewCAdESVerifier()
	res, err := v.VerifyRevocation(context.Background(), []*x509.Certificate{leaf, ca})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// Sin OCSP/CRL: no puede confirmar revocación, sigue siendo válido (degradado).
	if !res.Valid {
		t.Errorf("sin OCSP/CRL debería seguir siendo válido (degradado): %s", res.Reason)
	}
	if len(res.Details) == 0 {
		t.Error("esperado al menos un detalle")
	}
	if got := res.Details[0]; got == "" || strings.Contains(got, "vía ") {
		t.Errorf("detalle de revocación mal formateado: %q", got)
	}
}

// TestRevocationChecker_OCSP_EnviaRequest verifica que el checker envía petición OCSP al servidor.
// No verifica el parseo completo de la respuesta: eso requiere un TSP real o golang.org/x/crypto/ocsp.
func TestRevocationChecker_OCSP_EnviaRequest(t *testing.T) {
	t.Parallel()

	recibido := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recibido = true
		if r.Header.Get("Content-Type") != "application/ocsp-request" {
			t.Errorf("Content-Type esperado application/ocsp-request, obtenido %q", r.Header.Get("Content-Type"))
		}
		// Responder con un DER inválido para forzar fallback a CRL.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	leaf, ca, _ := generarCertChain(t)
	leaf.OCSPServer = []string{"http://93.184.216.34/ocsp"}

	checker := signer.NewRevocationCheckerWithClient(clienteProxyRevocacion(t, srv, 2*time.Second))
	// Con HTTP 500 el OCSP falla; Check retorna Unknown (CRL también vacía).
	checker.Check(context.Background(), leaf, ca) //nolint:errcheck

	if !recibido {
		t.Error("el checker no envió ninguna petición OCSP")
	}
}

// TestRevocationChecker_CRL_Mock verifica que el checker descarga y parsea una CRL.
func TestRevocationChecker_CRL_Mock(t *testing.T) {
	t.Parallel()

	leaf, ca, caKey := generarCertChain(t)

	crlTpl := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: time.Now().Add(-time.Minute),
		NextUpdate: time.Now().Add(24 * time.Hour),
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, crlTpl, ca, caKey)
	if err != nil {
		t.Fatalf("crear CRL: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pkix-crl")
		w.Write(crlDER)
	}))
	defer srv.Close()

	leaf.CRLDistributionPoints = []string{"http://93.184.216.34/crl"}
	// Sin OCSP para forzar fallback a CRL.
	leaf.OCSPServer = nil

	checker := signer.NewRevocationCheckerWithClient(clienteProxyRevocacion(t, srv, 2*time.Second))
	res, err := checker.Check(context.Background(), leaf, ca)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Method != "crl" {
		t.Errorf("esperado método crl, obtenido %q", res.Method)
	}
	if res.Status != signer.RevocationGood {
		t.Errorf("esperado RevocationGood, obtenido %v", res.Status)
	}
}

func TestRevocationChecker_CRL_Revocado(t *testing.T) {
	t.Parallel()

	leaf, ca, caKey := generarCertChain(t)

	crlTpl := &x509.RevocationList{
		Number:     big.NewInt(2),
		ThisUpdate: time.Now().Add(-time.Minute),
		NextUpdate: time.Now().Add(24 * time.Hour),
		RevokedCertificateEntries: []x509.RevocationListEntry{
			{
				SerialNumber:   leaf.SerialNumber,
				RevocationTime: time.Now().Add(-time.Minute),
			},
		},
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, crlTpl, ca, caKey)
	if err != nil {
		t.Fatalf("crear CRL revocada: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pkix-crl")
		_, _ = w.Write(crlDER)
	}))
	defer srv.Close()

	leaf.CRLDistributionPoints = []string{"http://93.184.216.34/crl"}
	leaf.OCSPServer = nil

	checker := signer.NewRevocationCheckerWithClient(clienteProxyRevocacion(t, srv, 2*time.Second))
	res, err := checker.Check(context.Background(), leaf, ca)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Status != signer.RevocationRevoked {
		t.Fatalf("esperado RevocationRevoked, obtenido %v", res.Status)
	}
	if res.Method != "crl" {
		t.Fatalf("esperado metodo crl, obtenido %q", res.Method)
	}
}

// TestRevocationChecker_HTTP500 verifica que un error HTTP se propaga correctamente.
func TestRevocationChecker_HTTP500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	leaf, ca, _ := generarCertChain(t)
	leaf.OCSPServer = []string{"http://93.184.216.34/ocsp"}
	leaf.CRLDistributionPoints = []string{"http://93.184.216.34/crl"}

	checker := signer.NewRevocationCheckerWithClient(clienteProxyRevocacion(t, srv, 2*time.Second))
	res, _ := checker.Check(context.Background(), leaf, ca)
	// Ambos fallan → Unknown (degraded, no error fatal)
	if res.Status != signer.RevocationUnknown {
		t.Errorf("esperado RevocationUnknown tras HTTP 500, obtenido %v", res.Status)
	}
}

func TestRevocationChecker_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	leaf, ca, _ := generarCertChain(t)
	leaf.OCSPServer = []string{"http://93.184.216.34/ocsp"}
	leaf.CRLDistributionPoints = nil

	checker := signer.NewRevocationCheckerWithClient(clienteProxyRevocacion(t, srv, 20*time.Millisecond))
	res, err := checker.Check(context.Background(), leaf, ca)
	if err != nil {
		t.Fatalf("Check() no debe propagar timeout fatal, obtuvo %v", err)
	}
	if res.Status != signer.RevocationUnknown {
		t.Fatalf("esperado RevocationUnknown tras timeout, obtenido %v", res.Status)
	}
}
