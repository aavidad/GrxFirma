// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package updatecheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type updateRoundTripper struct{}

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (*updateRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.Canceled
}

func TestEsMasNueva(t *testing.T) {
	casos := []struct {
		actual, ultima string
		quiere         bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"v1.2.3", "v1.2.3", false},
		{"1.2.3", "1.2.2", false},
		{"2.0.0", "1.9.9", false},
		{"1.2", "1.2.1", true},
		{"v0.9.3", "v0.10.0", true},
		{"1.0.0-rc1", "1.0.0", true},  // estable sucede a pre-release
		{"1.0.0", "1.0.0-rc1", false}, // no retroceder a pre-release
		{"dev", "1.0.0", false},       // actual no parseable => no afirmar
		{"48e00fb", "1.0.0", false},   // hash git => no afirmar
		{"1.0.0", "trunk", false},     // última no parseable
	}
	for _, c := range casos {
		if got := esMasNueva(c.actual, c.ultima); got != c.quiere {
			t.Errorf("esMasNueva(%q, %q) = %v, quiere %v", c.actual, c.ultima, got, c.quiere)
		}
	}
}

func TestComprobar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "GrxFirma-UpdateCheck" ||
			r.Header.Get("Accept") != "application/vnd.github+json" ||
			r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" ||
			r.Header.Get("Authorization") != "" {
			t.Errorf("cabeceras GitHub incompletas: %#v", r.Header)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.org/rel/v2.0.0"}`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	res, err := c.Comprobar(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatalf("Comprobar() error = %v", err)
	}
	if !res.HayNueva || !res.Comparable || res.UltimaVersion != "v2.0.0" || res.URL != "https://example.org/rel/v2.0.0" {
		t.Errorf("resultado inesperado: %+v", res)
	}
}

func TestComprobarBuildDesarrolloNoGeneraFalsoAviso(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.org/rel/v2.0.0"}`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	res, err := c.Comprobar(context.Background(), "dev")
	if err != nil {
		t.Fatalf("Comprobar() error = %v", err)
	}
	if res.HayNueva || res.Comparable {
		t.Fatalf("un build de desarrollo no se puede comparar: %+v", res)
	}
}

func TestComprobar404SoloEnCanalOficial(t *testing.T) {
	client := NewWithHTTPClient(&http.Client{Transport: updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != OfficialLatestAPIURL {
			t.Fatalf("endpoint inesperado: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
	})})
	result, err := client.Comprobar(context.Background(), "0.0.100")
	if err != nil || result.Estado != EstadoSinPublicaciones || result.HayNueva || result.Comparable || result.VersionActual != "0.0.100" || result.URL != "" {
		t.Fatalf("404 oficial = (%+v, %v)", result, err)
	}
	client = NewWithHTTPClient(&http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})})
	client.URL = "http://localhost:1234/releases/latest"
	if _, err := client.Comprobar(context.Background(), "0.0.100"); err == nil {
		t.Fatal("un 404 ajeno al endpoint oficial no debe convertirse en ausencia de publicaciones")
	}
}

func TestComprobar404TrasRedireccionNoEsSinPublicaciones(t *testing.T) {
	client := NewWithHTTPClient(&http.Client{Transport: updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/repos/aavidad/GrxFirma/releases/latest" {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"/otro-recurso"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})})
	if _, err := client.Comprobar(context.Background(), "0.0.100"); err == nil {
		t.Fatal("un 404 tras redirección no acredita ausencia de releases")
	}
}

func TestComprobarErroresDistinguidos(t *testing.T) {
	for _, test := range []struct {
		name          string
		status        int
		err           error
		code, message string
	}{
		{"limite", 403, nil, "update_rate_limited", "GitHub ha limitado"},
		{"servidor", 500, nil, "update_service_unavailable", "no está disponible"},
		{"sin_red", 0, errors.New("red caída: token=secreto host=dns.example"), "update_network_unavailable", "conectar"},
		{"timeout", 0, context.DeadlineExceeded, "update_timeout", "tardado demasiado"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewWithHTTPClient(&http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
				if test.err != nil {
					return nil, test.err
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})})
			_, err := client.Comprobar(context.Background(), "0.0.100")
			if err == nil || ErrorCode(err) != test.code || !strings.Contains(MessageKey(err), test.message) {
				t.Fatalf("error = %v, code = %s, message = %s", err, ErrorCode(err), MessageKey(err))
			}
			if strings.Contains(MessageKey(err), "secreto") || strings.Contains(MessageKey(err), "dns.example") {
				t.Fatal("el mensaje filtró datos de red")
			}
		})
	}
}

func TestNewWithHTTPClientConservaTransporteYAcotaTimeout(t *testing.T) {
	t.Parallel()
	transport := &updateRoundTripper{}
	base := &http.Client{
		Timeout:   2 * time.Minute,
		Transport: transport,
	}
	client := NewWithHTTPClient(base)
	if client.HTTP == base {
		t.Fatal("el comprobador no debe modificar el cliente compartido")
	}
	if client.HTTP.Transport != transport {
		t.Fatal("no se conservó el transporte/proxy compartido")
	}
	if client.HTTP.Timeout != defaultClientTimeout {
		t.Fatalf("timeout = %s; want %s", client.HTTP.Timeout, defaultClientTimeout)
	}
	if base.Timeout != 2*time.Minute {
		t.Fatalf("se modificó el timeout compartido: %s", base.Timeout)
	}
}

func TestFailureCodeNoFiltraElDetalleDeRed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "TIMEOUT"},
		{context.Canceled, "CANCELLED"},
		{errors.New("consultando releases: HTTP 404"), "HTTP 404"},
		{errors.New("respuesta HTTP 503 desde upstream"), "HTTP 503"},
		{errors.New(`proxy https://usuario:secreto@proxy.example rechazado`), "PROXY_UNAVAILABLE"},
		{&HTTPStatusError{StatusCode: http.StatusProxyAuthRequired}, "PROXY_AUTH_REQUIRED"},
		{errors.New(`Get "https://api.github.com/x": Proxy Authentication Required`), "PROXY_AUTH_REQUIRED"},
		{errors.New("HTTP abc; token=secreto"), "NETWORK_OR_PUBLICATION_UNAVAILABLE"},
	}
	for _, test := range cases {
		if got := FailureCode(test.err); got != test.want {
			t.Errorf("FailureCode(%v) = %q; want %q", test.err, got, test.want)
		}
	}
	proxyAuth := &HTTPStatusError{StatusCode: http.StatusProxyAuthRequired}
	if got := ErrorCode(proxyAuth); got != "update_proxy_auth_required" {
		t.Errorf("ErrorCode(407) = %q", got)
	}
	if got := MessageKey(proxyAuth); !strings.Contains(got, "proxy") {
		t.Errorf("MessageKey(407) = %q", got)
	}
}

func TestReleaseDestinationOficialQuedaAncladoAlRepositorio(t *testing.T) {
	endpoint, err := validateEndpoint(OfficialLatestAPIURL)
	if err != nil {
		t.Fatalf("endpoint oficial inválido: %v", err)
	}
	if _, err := releaseDestination(
		endpoint,
		"https://example.org/GrxFirma-actualizado.exe",
	); err == nil {
		t.Fatal("se aceptó un destino de release externo")
	}
	if _, err := releaseDestination(
		endpoint,
		"https://github.com/aavidad/GrxFirma/releases/download/v2.0.0/GrxFirma.exe",
	); err == nil {
		t.Fatal("se aceptó un enlace de descarga en vez de la página de la release")
	}
	// La etiqueta es un solo segmento literal: ni barras, ni «..», ni
	// secuencias escapadas que el navegador resolvería después.
	for _, malo := range []string{
		"https://github.com/aavidad/GrxFirma/releases/tag/v2.0.0/../../../../otro/repo",
		"https://github.com/aavidad/GrxFirma/releases/tag/%2e%2e/%2e%2e/%2e%2e/otro",
		"https://github.com/aavidad/GrxFirma/releases/tag/%2e%2e",
		"https://github.com/aavidad/GrxFirma/releases/tag/v2%2f..%2f..",
		"https://github.com/aavidad/GrxFirma/releases/tag/v2.0.0/",
		"https://github.com/aavidad/GrxFirma/releases/tag/..",
	} {
		if _, err := releaseDestination(endpoint, malo); err == nil {
			t.Errorf("se aceptó %q", malo)
		}
	}
	for _, bueno := range []string{
		"https://github.com/aavidad/GrxFirma/releases/tag/0.0.116",
		"https://github.com/aavidad/GrxFirma/releases/tag/v2.0.1-rc.1",
	} {
		if _, err := releaseDestination(endpoint, bueno); err != nil {
			t.Errorf("se rechazó %q: %v", bueno, err)
		}
	}
	got, err := releaseDestination(
		endpoint,
		"https://github.com/aavidad/GrxFirma/releases/tag/v2.0.0",
	)
	if err != nil {
		t.Fatalf("releaseDestination() error = %v", err)
	}
	if got != "https://github.com/aavidad/GrxFirma/releases/tag/v2.0.0" {
		t.Fatalf("destino oficial = %q", got)
	}
}

func TestComprobarRespuestasInvalidas(t *testing.T) {
	casos := []struct {
		nombre string
		status int
		cuerpo string
	}{
		{"http_500", http.StatusInternalServerError, ""},
		{"json_invalido", http.StatusOK, "{no-json"},
		{"sin_tag", http.StatusOK, `{"html_url":"x"}`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.cuerpo))
			}))
			defer srv.Close()
			cl := New()
			cl.URL = srv.URL
			if _, err := cl.Comprobar(context.Background(), "1.0.0"); err == nil {
				t.Error("se esperaba error")
			}
		})
	}
}

func TestHabilitadoPorDefectoApagado(t *testing.T) {
	t.Setenv(EnvHabilitar, "")
	if Habilitado() {
		t.Error("la comprobación de actualizaciones debe estar apagada por defecto")
	}
	t.Setenv(EnvHabilitar, "1")
	if !Habilitado() {
		t.Error("GRXFIRMA_CHECK_UPDATES=1 debe habilitarla")
	}
}
