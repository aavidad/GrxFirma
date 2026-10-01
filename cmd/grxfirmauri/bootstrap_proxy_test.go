// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/tsatest"
)

func TestConstruirTSAClienteRuntime_ReutilizaClienteCompartido(t *testing.T) {
	t.Parallel()

	shared := &http.Client{Timeout: 11}
	tsa := construirTSAClienteRuntime(" https://tsa.ejemplo.test ", shared)
	if tsa == nil {
		t.Fatal("tsa nil")
	}
	if tsa.URL != "https://tsa.ejemplo.test" {
		t.Fatalf("URL = %q, want https://tsa.ejemplo.test", tsa.URL)
	}
	if tsa.HTTPClient != shared {
		t.Fatalf("HTTPClient inesperado: %#v", tsa.HTTPClient)
	}
}

func TestConstruirRevocationProviderRuntime_ReutilizaClienteCompartido(t *testing.T) {
	t.Parallel()

	shared := &http.Client{Timeout: 13}
	provider := construirRevocationProviderRuntime(shared)
	if provider == nil {
		t.Fatal("provider nil")
	}
	if _, ok := interface{}(provider).(*revocationclient.Client); !ok {
		t.Fatalf("provider inesperado: %T", provider)
	}
}

func TestConstruirClienteHTTPRuntimeSeguro_ReutilizaTransporteSeguro(t *testing.T) {
	enabled := false
	originalLoadSettings := usersettingsLoaderForProxyTests
	originalSecretStore := secretStoreForProxyTests
	usersettingsLoaderForProxyTests = func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled: &enabled,
		}), nil
	}
	secretStoreForProxyTests = &proxySecretLoaderMock{}
	defer func() {
		usersettingsLoaderForProxyTests = originalLoadSettings
		secretStoreForProxyTests = originalSecretStore
	}()

	client := construirClienteHTTPRuntimeSeguro("/tmp/config")
	if client == nil || client.Transport == nil {
		t.Fatalf("client/transport inesperado: %#v", client)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport inesperado: %T", client.Transport)
	}
	req, err := http.NewRequest(http.MethodGet, "http://portal.ejemplo/rt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if transport.Proxy != nil {
		proxyURL, err := transport.Proxy(req)
		if err != nil {
			t.Fatalf("Proxy() error = %v", err)
		}
		if proxyURL != nil {
			t.Fatalf("se esperaba sin proxy para cliente compartido seguro, got %#v", proxyURL)
		}
	}
}

func TestResolverClienteHTTPRuntime_UsaClienteExistenteSinReleerSettings(t *testing.T) {
	sentinel := &http.Client{Timeout: 17}
	originalLoadSettings := usersettingsLoaderForProxyTests
	loadCalls := 0
	usersettingsLoaderForProxyTests = func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		loadCalls++
		return ports.DocumentoConfiguracionUsuario{}, nil
	}
	defer func() {
		usersettingsLoaderForProxyTests = originalLoadSettings
	}()

	got := resolverClienteHTTPRuntime("/tmp/config", sentinel)
	if got != sentinel {
		t.Fatalf("resolverClienteHTTPRuntime() = %#v, want sentinel", got)
	}
	if loadCalls != 0 {
		t.Fatalf("no deberia releer settings si ya hay cliente compartido: %d", loadCalls)
	}
}

func TestConstruirTSAClienteRuntime_E2EUsaProxySeguroCompartido(t *testing.T) {
	t.Parallel()

	var (
		proxySeen bool
		authSeen  string
		urlSeen   string
	)
	targetURL := "http://tsa.ejemplo.local/ts"
	tsaResponder := tsatest.NewResponder(t)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxySeen = true
		authSeen = r.Header.Get("Proxy-Authorization")
		urlSeen = r.URL.String()
		tsaResponder.ServeHTTP(w, r)
	}))
	defer proxySrv.Close()

	host, port := splitProxyHostPort(t, proxySrv.URL)
	enabled := true
	typ := "manual"
	secretID := "secret-1"
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled:  &enabled,
			Type:     &typ,
			Host:     &host,
			Port:     &port,
			SecretID: &secretID,
		}), nil
	}
	secrets := &proxySecretLoaderMock{
		status: ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  "linux",
			Backend:   "secret-service",
		},
		material: ports.ProxySecretMaterial{
			Username: "alberto",
			Password: []byte("secreto"),
		},
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, secrets)
	tsa := construirTSAClienteRuntime(targetURL, client)
	hash := crypto.SHA256.New().Sum(nil)
	if _, err := tsa.RequestTimestamp(context.Background(), hash, crypto.SHA256); err != nil {
		t.Fatalf("RequestTimestamp() error = %v", err)
	}

	if !proxySeen {
		t.Fatal("se esperaba trafico TSA por el proxy seguro compartido")
	}
	if urlSeen != targetURL {
		t.Fatalf("URL vista por proxy = %q, want %q", urlSeen, targetURL)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alberto:secreto"))
	if authSeen != wantAuth {
		t.Fatalf("Proxy-Authorization = %q, want %q", authSeen, wantAuth)
	}
}

func TestConstruirRevocationProviderRuntime_E2EUsaProxySeguroCompartido(t *testing.T) {
	t.Parallel()

	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA proxy revocación"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(
		rand.Reader,
		caTemplate,
		caTemplate,
		&caKey.PublicKey,
		caKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Firmante proxy revocación"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
	}, issuer, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: now.Add(-time.Minute),
		NextUpdate: now.Add(time.Hour),
	}, issuer, caKey)
	if err != nil {
		t.Fatal(err)
	}

	var (
		proxySeen bool
		authSeen  string
		urlSeen   string
	)
	targetURL := "http://8.8.8.8/list.crl"
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxySeen = true
		authSeen = r.Header.Get("Proxy-Authorization")
		urlSeen = r.URL.String()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(crlDER)
	}))
	defer proxySrv.Close()

	host, port := splitProxyHostPort(t, proxySrv.URL)
	enabled := true
	typ := "manual"
	secretID := "secret-1"
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled:  &enabled,
			Type:     &typ,
			Host:     &host,
			Port:     &port,
			SecretID: &secretID,
		}), nil
	}
	secrets := &proxySecretLoaderMock{
		status: ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  "windows",
			Backend:   "credential-manager-dpapi",
		},
		material: ports.ProxySecretMaterial{
			Username: "alberto",
			Password: []byte("secreto"),
		},
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, secrets)
	provider := construirRevocationProviderRuntime(client)

	leaf.CRLDistributionPoints = []string{targetURL}
	evidence, err := provider.Fetch(context.Background(), leaf, issuer)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(evidence.CRLs) != 1 {
		t.Fatalf("CRLs = %d, want 1", len(evidence.CRLs))
	}
	if !proxySeen {
		t.Fatal("se esperaba trafico de revocacion por el proxy seguro compartido")
	}
	if urlSeen != targetURL {
		t.Fatalf("URL vista por proxy = %q, want %q", urlSeen, targetURL)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alberto:secreto"))
	if authSeen != wantAuth {
		t.Fatalf("Proxy-Authorization = %q, want %q", authSeen, wantAuth)
	}
}

func splitProxyHostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	trimmed := strings.TrimPrefix(raw, "http://")
	host, portText, ok := strings.Cut(trimmed, ":")
	if !ok {
		t.Fatalf("proxy URL inesperada: %q", raw)
	}
	port := 0
	for _, ch := range portText {
		if ch < '0' || ch > '9' {
			t.Fatalf("puerto proxy inesperado: %q", portText)
		}
		port = port*10 + int(ch-'0')
	}
	return host, port
}
