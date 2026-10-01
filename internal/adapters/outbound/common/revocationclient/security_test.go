// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// El bypass para loopback solo existe en el binario de pruebas y permite usar
// servidores httptest sin debilitar los constructores de producción.
func testEndpointPolicy() endpointPolicy {
	policy := defaultEndpointPolicy()
	policy.allowPrivate = true
	return policy
}

func TestEndpointPolicy_ValidaURLYDirecciones(t *testing.T) {
	publicLookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	privateLookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("172.16.1.20")}}, nil
	}
	mixedLookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("8.8.8.8")},
			{IP: net.ParseIP("127.0.0.1")},
		}, nil
	}

	tests := []struct {
		name    string
		rawURL  string
		lookup  lookupIPFunc
		wantErr string
	}{
		{name: "HTTP público", rawURL: "http://ocsp.example.test/status", lookup: publicLookup},
		{name: "HTTPS público", rawURL: "https://8.8.8.8/crl"},
		{name: "esquema file", rawURL: "file:///etc/passwd", wantErr: "esquema URL no permitido"},
		{name: "esquema LDAP", rawURL: "ldap://ca.example.test/crl", wantErr: "esquema URL no permitido"},
		{name: "URL relativa", rawURL: "/ocsp", wantErr: "esquema URL no permitido"},
		{name: "credenciales", rawURL: "https://user:secret@ocsp.example.test/", lookup: publicLookup, wantErr: "credenciales"},
		{name: "loopback IPv4", rawURL: "http://127.0.0.1/", wantErr: "loopback"},
		{name: "loopback IPv6", rawURL: "http://[::1]/", wantErr: "loopback"},
		{name: "privada IPv4", rawURL: "http://172.16.10.10/", wantErr: "privada"},
		{name: "privada IPv6", rawURL: "http://[fd00::1]/", wantErr: "privada"},
		{name: "CGNAT", rawURL: "http://100.100.100.200/", wantErr: "CGNAT"},
		{name: "link-local IPv4", rawURL: "http://169.254.169.254/", wantErr: "link-local"},
		{name: "link-local IPv6", rawURL: "http://[fe80::1]/", wantErr: "link-local"},
		{name: "DNS privado", rawURL: "https://internal.example.test/", lookup: privateLookup, wantErr: "privada"},
		{name: "DNS mixto", rawURL: "https://mixed.example.test/", lookup: mixedLookup, wantErr: "loopback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.rawURL, err)
			}
			policy := endpointPolicy{lookupIP: tt.lookup}
			err = policy.validateEndpoint(context.Background(), endpoint)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateEndpoint() error inesperado: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateEndpoint() error=%v, se esperaba %q", err, tt.wantErr)
			}
		})
	}
}

func TestEndpointPolicy_BloqueaRangosEspecialesYTransicion(t *testing.T) {
	t.Parallel()
	policy := endpointPolicy{}
	blocked := []string{
		"0.1.2.3",
		"192.0.0.1",
		"192.0.2.1",
		"192.88.99.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"255.255.255.255",
		"::192.0.2.1",
		"::ffff:127.0.0.1",
		"64:ff9b::c000:201",
		"64:ff9b:1::1",
		"100::1",
		"2001::1",
		"2001:db8::1",
		"2002:c000:201::1",
		"3fff::1",
		"5f00::1",
		"fec0::1",
	}
	for _, rawIP := range blocked {
		t.Run(rawIP, func(t *testing.T) {
			t.Parallel()
			if err := policy.validateIP(net.ParseIP(rawIP)); err == nil {
				t.Fatalf("validateIP(%q) aceptó una dirección especial", rawIP)
			}
		})
	}
	for _, rawIP := range []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"} {
		t.Run("pública_"+rawIP, func(t *testing.T) {
			t.Parallel()
			if err := policy.validateIP(net.ParseIP(rawIP)); err != nil {
				t.Fatalf("validateIP(%q) rechazó una dirección pública: %v", rawIP, err)
			}
		})
	}
}

func TestEndpointPolicy_RevalidaDNSAlConectar(t *testing.T) {
	var lookups atomic.Int32
	policy := endpointPolicy{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			if lookups.Add(1) == 1 {
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
	}
	endpoint, err := url.Parse("https://rebind.example.test/status")
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.validateEndpoint(context.Background(), endpoint); err != nil {
		t.Fatalf("la validación inicial debería aceptar la IP pública: %v", err)
	}

	dialCalled := false
	dial := policy.secureDialContext(func(context.Context, string, string) (net.Conn, error) {
		dialCalled = true
		return nil, errors.New("no debería ejecutarse")
	})
	_, err = dial(context.Background(), "tcp", "rebind.example.test:443")
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("secureDialContext() error=%v, se esperaba bloqueo loopback", err)
	}
	if dialCalled {
		t.Fatal("se intentó conectar después de que el DNS cambiara a una IP bloqueada")
	}
}

func TestEndpointPolicy_ConectaConLaIPPublicaResuelta(t *testing.T) {
	policy := endpointPolicy{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
	}
	sentinel := errors.New("dial simulado")
	var gotAddress string
	dial := policy.secureDialContext(func(_ context.Context, _, address string) (net.Conn, error) {
		gotAddress = address
		return nil, sentinel
	})
	_, err := dial(context.Background(), "tcp", "ocsp.example.test:80")
	if !errors.Is(err, sentinel) {
		t.Fatalf("secureDialContext() error=%v, se esperaba %v", err, sentinel)
	}
	if gotAddress != "8.8.8.8:80" {
		t.Fatalf("destino efectivo=%q, se esperaba la IP resuelta", gotAddress)
	}
}

func TestHardenHTTPClient_PermiteRedirectMismoOrigen(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inicio" {
			http.Redirect(w, r, server.URL+"/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := hardenHTTPClient(server.Client(), testEndpointPolicy())
	resp, err := client.Get(server.URL + "/inicio")
	if err != nil {
		t.Fatalf("redirect del mismo origen bloqueado: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestHardenHTTPClient_BloqueaRedirectAOtroOrigen(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("el cliente alcanzó el origen de destino bloqueado")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/crl", http.StatusFound)
	}))
	defer source.Close()

	client := hardenHTTPClient(source.Client(), testEndpointPolicy())
	_, err := client.Get(source.URL + "/inicio")
	if err == nil || !strings.Contains(err.Error(), "otro origen") {
		t.Fatalf("Get() error=%v, se esperaba bloqueo por cambio de origen", err)
	}
}

func TestHardenHTTPClient_BloqueaDowngradeHTTPS(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("el cliente alcanzó el destino HTTP de un downgrade")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/crl", http.StatusFound)
	}))
	defer source.Close()

	client := hardenHTTPClient(source.Client(), testEndpointPolicy())
	_, err := client.Get(source.URL + "/inicio")
	if err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("Get() error=%v, se esperaba bloqueo de downgrade", err)
	}
}

func TestEndpointPolicy_BloqueaRedirectARedPrivada(t *testing.T) {
	policy := endpointPolicy{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
	}
	previous, err := http.NewRequest(http.MethodGet, "http://ocsp.example.test/inicio", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = policy.redirectPolicy(nil)(redirect, []*http.Request{previous})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("redirectPolicy() error=%v, se esperaba bloqueo de red privada", err)
	}
}

func TestHardenHTTPClient_PermiteProxyPrivadoConfigurado(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "ocsp.example.test" {
			t.Errorf("Host recibido por el proxy=%q", r.Host)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	policy := endpointPolicy{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
	}
	client := hardenHTTPClient(&http.Client{Transport: transport}, policy)

	resp, err := client.Get("http://ocsp.example.test/crl")
	if err != nil {
		t.Fatalf("el proxy explícitamente configurado debería ser utilizable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestHardenHTTPClient_RechazaRoundTripperNoAuditable(t *testing.T) {
	client := hardenHTTPClient(&http.Client{Transport: stubRoundTripper{}}, testEndpointPolicy())
	_, err := client.Get("https://8.8.8.8/ocsp")
	if err == nil || !strings.Contains(err.Error(), "transporte HTTP personalizado no soportado") {
		t.Fatalf("Get() error=%v, se esperaba fallo cerrado", err)
	}
}

type stubRoundTripper struct{}

func (stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no debería ejecutarse")
}
