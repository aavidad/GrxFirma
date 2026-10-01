// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestValidateEndpoint_ExigeHTTPSFueraDeLoopback(t *testing.T) {
	t.Parallel()

	cases := []string{
		"http://example.com/releases/latest",
		"http://192.0.2.10/releases/latest",
		"ftp://example.com/releases/latest",
		"https://user:secret@example.com/releases/latest",
		"https://example.com/releases/latest#fragment",
	}
	for _, endpoint := range cases {
		if _, err := validateEndpoint(endpoint); err == nil {
			t.Errorf("validateEndpoint(%q) debe rechazar el endpoint", endpoint)
		}
	}
}

func TestValidateEndpoint_AdmiteHTTPSYHTTPLoopback(t *testing.T) {
	t.Parallel()

	cases := []string{
		"https://example.com/releases/latest",
		"http://localhost:8080/releases/latest",
		"http://127.0.0.1:8080/releases/latest",
		"http://[::1]:8080/releases/latest",
	}
	for _, endpoint := range cases {
		if _, err := validateEndpoint(endpoint); err != nil {
			t.Errorf("validateEndpoint(%q) error = %v", endpoint, err)
		}
	}
}

func TestCheck_AdmiteRedireccionMismoOrigen(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inicio" {
			http.Redirect(w, r, "/latest", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.org/v2.0.0"}`))
	}))
	defer srv.Close()

	result, newer, err := Check(context.Background(), "v1.0.0", srv.URL+"/inicio")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !newer || result.Version != "v2.0.0" {
		t.Fatalf("Check() = (%+v, %v), want version nueva", result, newer)
	}
}

func TestCheck_RechazaRedireccionAOtroOrigen(t *testing.T) {
	t.Parallel()

	var destinationHits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationHits.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	}))
	defer destination.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()

	if _, _, err := Check(context.Background(), "v1.0.0", source.URL); err == nil {
		t.Fatal("Check() debe rechazar una redireccion a otro origen")
	}
	if got := destinationHits.Load(); got != 0 {
		t.Fatalf("el destino no confiable recibio %d peticiones, want 0", got)
	}
}

func TestComprobar_RechazaDowngradeHTTPS(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+"/latest", http.StatusFound)
	}))
	defer srv.Close()

	client := New()
	client.URL = srv.URL
	client.HTTP = srv.Client()
	if _, err := client.Comprobar(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("Comprobar() debe rechazar una redireccion de HTTPS a HTTP")
	}
}

func TestCheck_RechazaRespuestaDemasiadoGrande(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","padding":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxBodyBytes))))
		_, _ = w.Write([]byte(`"}`))
	}))
	defer srv.Close()

	_, _, err := Check(context.Background(), "v1.0.0", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "demasiado grande") {
		t.Fatalf("Check() error = %v, want respuesta demasiado grande", err)
	}
}

func TestComprobar_RechazaHTTPRemotoAntesDeConectar(t *testing.T) {
	t.Parallel()

	client := New()
	client.URL = "http://example.com/releases/latest"
	if _, err := client.Comprobar(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("Comprobar() debe rechazar HTTP remoto")
	}
}
