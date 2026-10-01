// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

type proxySecretLoaderMock struct {
	calls    []string
	material ports.ProxySecretMaterial
	err      error
	status   ports.ProxySecretStoreStatus
}

func (m *proxySecretLoaderMock) Store(context.Context, string, ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	return ports.ProxySecretDescriptor{}, errors.New("not implemented")
}

func (m *proxySecretLoaderMock) Load(_ context.Context, id string) (ports.ProxySecretMaterial, error) {
	m.calls = append(m.calls, id)
	if m.err != nil {
		return ports.ProxySecretMaterial{}, m.err
	}
	return m.material, nil
}

func (m *proxySecretLoaderMock) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func (m *proxySecretLoaderMock) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	if m.status.Platform == "" && m.status.Backend == "" && !m.status.Available {
		return ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  "linux",
			Backend:   "secret-service",
		}, nil
	}
	return m.status, nil
}

func docWithProxy(proxy ports.ConfiguracionUsuarioProxy) ports.DocumentoConfiguracionUsuario {
	return ports.DocumentoConfiguracionUsuario{Proxy: proxy}
}

func TestConstruirClienteTrifasicoConDependencias_ManualConSecreto(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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
	if client == nil || client.Transport == nil {
		t.Fatalf("client/transport inesperado: %#v", client)
	}
	req, err := http.NewRequest(http.MethodGet, "https://portal.ejemplo/servlet", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport inesperado: %T", client.Transport)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if proxyURL == nil || proxyURL.Host != "proxy.local:3128" {
		t.Fatalf("proxy URL inesperada: %#v", proxyURL)
	}
	if proxyURL.User == nil {
		t.Fatal("proxy URL sin credenciales")
	}
	if user := proxyURL.User.Username(); user != "alberto" {
		t.Fatalf("username proxy inesperado: %q", user)
	}
	pass, ok := proxyURL.User.Password()
	if !ok || pass != "secreto" {
		t.Fatalf("password proxy inesperado: %q %v", pass, ok)
	}
	if len(secrets.calls) != 1 || secrets.calls[0] != "secret-1" {
		t.Fatalf("calls inesperadas: %#v", secrets.calls)
	}
}

func TestConstruirTransporteHTTPCompartidoConDependencias_UsaProxySeguro(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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

	transport := construirTransporteHTTPCompartidoConDependencias("/tmp/config", loader, secrets)
	req, err := http.NewRequest(http.MethodGet, "https://portal.ejemplo/servlet", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if proxyURL == nil || proxyURL.Host != "proxy.local:3128" {
		t.Fatalf("proxy URL inesperada: %#v", proxyURL)
	}
}

func TestConstruirClienteTrifasicoConDependencias_BypassExcluded(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled:      &enabled,
			Type:         &typ,
			Host:         &host,
			Port:         &port,
			ExcludedURLs: []string{"*.dipgra.es", "https://intra.local"},
		}), nil
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, &proxySecretLoaderMock{})
	transport := client.Transport.(*http.Transport)

	req1, _ := http.NewRequest(http.MethodGet, "https://servicio.dipgra.es/rt", nil)
	proxyURL, err := transport.Proxy(req1)
	if err != nil {
		t.Fatalf("Proxy(req1) error = %v", err)
	}
	if proxyURL != nil {
		t.Fatalf("se esperaba bypass para *.dipgra.es, got %#v", proxyURL)
	}

	req2, _ := http.NewRequest(http.MethodGet, "https://portal.ejemplo/rt", nil)
	proxyURL, err = transport.Proxy(req2)
	if err != nil {
		t.Fatalf("Proxy(req2) error = %v", err)
	}
	if proxyURL == nil || proxyURL.Host != "proxy.local:3128" {
		t.Fatalf("proxy URL inesperada: %#v", proxyURL)
	}
}

func TestConstruirClienteTrifasicoConDependencias_BypassLoopback(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled: &enabled,
			Type:    &typ,
			Host:    &host,
			Port:    &port,
		}), nil
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, &proxySecretLoaderMock{})
	transport := client.Transport.(*http.Transport)

	for _, raw := range []string{
		"http://localhost:8080/rt",
		"http://127.0.0.1:8080/rt",
		"http://[::1]:8080/rt",
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		proxyURL, err := transport.Proxy(req)
		if err != nil {
			t.Fatalf("Proxy(%s) error = %v", raw, err)
		}
		if proxyURL != nil {
			t.Fatalf("se esperaba bypass loopback para %s, got %#v", raw, proxyURL)
		}
	}
}

func TestConstruirTransporteHTTPCompartidoConDependencias_ProxyDisabledNoUsaEntorno(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://envproxy.local:8080")
	enabled := false
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled: &enabled,
		}), nil
	}

	transport := construirTransporteHTTPCompartidoConDependencias("/tmp/config", loader, &proxySecretLoaderMock{})
	req, err := http.NewRequest(http.MethodGet, "http://portal.ejemplo/rt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if transport.Proxy != nil {
		if proxyURL, err := transport.Proxy(req); err != nil {
			t.Fatalf("Proxy() error = %v", err)
		} else if proxyURL != nil {
			t.Fatalf("se esperaba sin proxy aunque exista HTTP_PROXY, got %#v", proxyURL)
		}
	}
}

func TestConstruirTransporteHTTPCompartidoConDependencias_ProxySystemMantieneEntorno(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://envproxy.local:8080")
	enabled := true
	typ := "system"
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled: &enabled,
			Type:    &typ,
		}), nil
	}

	transport := construirTransporteHTTPCompartidoConDependencias("/tmp/config", loader, &proxySecretLoaderMock{})
	req, err := http.NewRequest(http.MethodGet, "http://portal.ejemplo/rt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if proxyURL == nil || proxyURL.Host != "envproxy.local:8080" {
		t.Fatalf("se esperaba proxy del entorno, got %#v", proxyURL)
	}
}

func TestConstruirClienteTrifasicoConDependencias_ErrorDeSettingsFallaCerrado(t *testing.T) {
	t.Parallel()

	client := construirClienteTrifasicoConDependencias("/tmp/config", func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return ports.DocumentoConfiguracionUsuario{}, context.DeadlineExceeded
	}, &proxySecretLoaderMock{})
	if client == nil {
		t.Fatal("client nil")
	}
	if client.Timeout.Seconds() != 30 {
		t.Fatalf("timeout inesperado: %v", client.Timeout)
	}
	if client.Transport == nil {
		t.Fatal("transport nil inesperado")
	}
	transport := client.Transport.(*http.Transport)
	req, err := http.NewRequest(http.MethodGet, "https://portal.ejemplo/rt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if !errors.Is(err, errProxyRuntimeFailClosed) || proxyURL != nil {
		t.Fatalf("Proxy() = %#v, %v; debe fallar cerrado", proxyURL, err)
	}
}

func TestConstruirClienteTrifasicoConDependencias_FallaCerradoSiSecretStoreNoDisponible(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	realm := "corp-proxy"
	loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return docWithProxy(ports.ConfiguracionUsuarioProxy{
			Enabled:  &enabled,
			Type:     &typ,
			Host:     &host,
			Port:     &port,
			SecretID: &secretID,
			Realm:    &realm,
		}), nil
	}
	secrets := &proxySecretLoaderMock{
		status: ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  "windows",
			Backend:   "dpapi",
			Reason:    "proxysecretstore: backend DPAPI/Credential Manager pendiente de implementacion en Windows",
		},
		err: errors.New("secret backend failure"),
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, secrets)
	transport := client.Transport.(*http.Transport)

	req, _ := http.NewRequest(http.MethodGet, "https://portal.ejemplo/rt", nil)
	proxyURL, err := transport.Proxy(req)
	if !errors.Is(err, errProxyRuntimeFailClosed) || proxyURL != nil {
		t.Fatalf("Proxy() = %#v, %v; debe fallar cerrado", proxyURL, err)
	}
	if len(secrets.calls) != 0 {
		t.Fatalf("no deberia intentar cargar secretos con backend no disponible: %#v", secrets.calls)
	}
}

func TestConstruirClienteTrifasicoConDependencias_NoDegradaASinCredencialesSiFallaCargaDelSecreto(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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
		err: errors.New("secret backend failure"),
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, secrets)
	transport := client.Transport.(*http.Transport)

	req, _ := http.NewRequest(http.MethodGet, "https://portal.ejemplo/rt", nil)
	proxyURL, err := transport.Proxy(req)
	if !errors.Is(err, errProxyRuntimeFailClosed) || proxyURL != nil {
		t.Fatalf("Proxy() = %#v, %v; debe fallar cerrado", proxyURL, err)
	}
	if len(secrets.calls) != 1 || secrets.calls[0] != "secret-1" {
		t.Fatalf("calls inesperadas: %#v", secrets.calls)
	}
}

func TestConstruirTransporteHTTPCompartidoConDependencias_ErrorDeSecretoFallaCerradoSinEntorno(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://envproxy.local:8080")
	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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
		err: errors.New("secret backend failure"),
	}

	transport := construirTransporteHTTPCompartidoConDependencias("/tmp/config", loader, secrets)
	req, err := http.NewRequest(http.MethodGet, "http://portal.ejemplo/rt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if proxyURL, err := transport.Proxy(req); !errors.Is(err, errProxyRuntimeFailClosed) || proxyURL != nil {
		t.Fatalf("Proxy() = %#v, %v; se esperaba fail-closed", proxyURL, err)
	}
}

func TestDiagnosticarClienteHTTPRuntimeSeguroConDependencias_ManualSecureStore(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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

	diag := diagnosticarClienteHTTPRuntimeSeguroConDependencias("/tmp/config", loader, secrets)
	if diag.Mode != runtimeProxyModeManualSecureStore {
		t.Fatalf("mode = %q, want %q", diag.Mode, runtimeProxyModeManualSecureStore)
	}
	if diag.Host != "proxy.local" || diag.Port != 3128 || diag.SecretID != "secret-1" {
		t.Fatalf("diagnostic inesperado: %+v", diag)
	}
}

func TestDiagnosticarClienteHTTPRuntimeSeguroConDependencias_BackendNoDisponibleFallaCerrado(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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
			Available: false,
			Platform:  "windows",
			Backend:   "credential-manager-dpapi",
			Reason:    "backend no disponible",
		},
	}

	diag := diagnosticarClienteHTTPRuntimeSeguroConDependencias("/tmp/config", loader, secrets)
	if diag.Mode != runtimeProxyModeFailClosed {
		t.Fatalf("mode = %q, want %q", diag.Mode, runtimeProxyModeFailClosed)
	}
	if !strings.Contains(diag.Reason, "credential-manager-dpapi") {
		t.Fatalf("reason = %q, want backend visible", diag.Reason)
	}
}

func TestDiagnosticarClienteHTTPRuntimeSeguroConDependencias_FailClosed(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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
		err: errors.New("cannot decrypt"),
	}

	diag := diagnosticarClienteHTTPRuntimeSeguroConDependencias("/tmp/config", loader, secrets)
	if diag.Mode != runtimeProxyModeFailClosed {
		t.Fatalf("mode = %q, want %q", diag.Mode, runtimeProxyModeFailClosed)
	}
	if !strings.Contains(diag.Reason, "cannot decrypt") {
		t.Fatalf("reason = %q, want decrypt error", diag.Reason)
	}
}

func TestFormatRuntimeProxyDiagnostic(t *testing.T) {
	t.Parallel()

	got := formatRuntimeProxyDiagnostic(runtimeProxyDiagnostic{
		Mode:     runtimeProxyModeManualSecureStore,
		Type:     "manual",
		Host:     "proxy.local",
		Port:     3128,
		SecretID: "secret-1",
		Reason:   "ok",
	})
	for _, needle := range []string{"mode=manual-secure-store", "type=manual", "endpoint=proxy.local:3128", "secret=store", "reason=ok"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("diagnostic = %q, want substring %q", got, needle)
		}
	}
}

func TestConstruirClienteTrifasicoConDependencias_E2EUsaProxySeguro(t *testing.T) {
	t.Parallel()

	var (
		proxySeen bool
		authSeen  string
		urlSeen   string
	)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxySeen = true
		authSeen = r.Header.Get("Proxy-Authorization")
		urlSeen = r.URL.String()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxySrv.Close()

	enabled := true
	typ := "manual"
	proxyURL := strings.TrimPrefix(proxySrv.URL, "http://")
	host, portText, ok := strings.Cut(proxyURL, ":")
	if !ok {
		t.Fatalf("proxySrv URL inesperada: %q", proxySrv.URL)
	}
	port := 0
	for _, ch := range portText {
		if ch < '0' || ch > '9' {
			t.Fatalf("puerto proxy inesperado: %q", portText)
		}
		port = port*10 + int(ch-'0')
	}
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
	req, err := http.NewRequest(http.MethodGet, "http://servicio.ejemplo.local/rt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	_ = resp.Body.Close()

	if !proxySeen {
		t.Fatal("se esperaba trafico HTTP real por el proxy seguro")
	}
	if urlSeen != "http://servicio.ejemplo.local/rt" {
		t.Fatalf("URL vista por proxy = %q", urlSeen)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alberto:secreto"))
	if authSeen != wantAuth {
		t.Fatalf("Proxy-Authorization = %q, want %q", authSeen, wantAuth)
	}
}

func TestConstruirClienteTrifasicoConDependencias_E2EFailClosedNoUsaProxyEntorno(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")

	var envProxySeen bool
	envProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envProxySeen = true
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer envProxy.Close()
	t.Setenv("HTTP_PROXY", envProxy.URL)

	var targetSeen bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetSeen = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
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
		err: errors.New("cannot decrypt"),
	}

	client := construirClienteTrifasicoConDependencias("/tmp/config", loader, secrets)
	resp, err := client.Get(target.URL + "/rt")
	if !errors.Is(err, errProxyRuntimeFailClosed) {
		t.Fatalf("client.Get() error = %v, want errProxyRuntimeFailClosed", err)
	}
	if resp != nil {
		_ = resp.Body.Close()
		t.Fatalf("client.Get() response inesperada: %#v", resp)
	}

	if envProxySeen {
		t.Fatal("no deberia usar el proxy del entorno en modo fail-closed")
	}
	if targetSeen {
		t.Fatal("no debe conectar directamente al destino al fallar cerrado")
	}
}
