// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tsaclient

import (
	"context"
	"crypto"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/testsupport/tsatest"
)

func TestTimestampRedirectPolicy(t *testing.T) {
	origin, _ := http.NewRequest(http.MethodPost, "https://tsa.example/start", nil)
	for _, tc := range []struct {
		name, target, method string
		allowed              bool
	}{
		{"mismo origen", "https://tsa.example/final", http.MethodPost, true},
		{"otro origen", "https://other.example/final", http.MethodPost, false},
		{"otro puerto", "https://tsa.example:8443/final", http.MethodPost, false},
		{"downgrade", "http://tsa.example/final", http.MethodPost, false},
		{"convertido GET", "https://tsa.example/final", http.MethodGet, false},
		{"credenciales", "https://user:secret@tsa.example/final", http.MethodPost, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, tc.target, nil)
			if err := boundedHTTPClient(nil).CheckRedirect(req, []*http.Request{origin}); (err == nil) != tc.allowed {
				t.Fatalf("permitido=%v error=%v", tc.allowed, err)
			}
		})
	}
}

func TestTimestampClientPreservesConfiguration(t *testing.T) {
	want := errors.New("política administrada")
	configured := &http.Client{Timeout: time.Second, Transport: http.DefaultTransport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return want }}
	client := boundedHTTPClient(configured)
	req, _ := http.NewRequest(http.MethodPost, "https://tsa.example/final", nil)
	if client == configured || client.Transport != configured.Transport || client.Timeout != time.Second ||
		!errors.Is(client.CheckRedirect(req, []*http.Request{req}), want) {
		t.Fatal("se perdió o modificó la configuración administrada")
	}
	for _, timeout := range []time.Duration{0, -time.Second, time.Hour} {
		configured.Timeout = timeout
		if boundedHTTPClient(configured).Timeout != timestampRequestTimeout || configured.Timeout != timeout {
			t.Fatal("límite por defecto ausente o mutación del cliente compartido")
		}
	}
}

func TestTimestampRejectsCrossOriginBeforeSending(t *testing.T) {
	var calls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/must-not-be-called", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	hash := sha256.Sum256([]byte("prueba sintética"))
	_, err := New(origin.URL).RequestTimestamp(context.Background(), hash[:], crypto.SHA256)
	if err == nil || calls.Load() != 0 {
		t.Fatalf("err=%v peticiones externas=%d", err, calls.Load())
	}
}

func TestTimestampPreservesPOSTThroughSameOriginRedirect(t *testing.T) {
	responder := tsatest.NewResponder(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
			return
		}
		responder.ServeHTTP(w, r)
	}))
	defer server.Close()
	hash := sha256.Sum256([]byte("prueba sintética"))
	if _, err := New(server.URL+"/start").RequestTimestamp(context.Background(), hash[:], crypto.SHA256); err != nil {
		t.Fatal(err)
	}
}

func TestTimestampDoesNotDiscloseEndpointSecrets(t *testing.T) {
	client := New("https://tsa.example/?token=private-marker")
	client.HTTPClient = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "POST", URL: r.URL.String(), Err: errors.New("private-marker")}
	})}
	hash := sha256.Sum256([]byte("prueba sintética"))
	_, err := client.RequestTimestamp(context.Background(), hash[:], crypto.SHA256)
	if err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatalf("error no redactado: %v", err)
	}
}

func TestTimestampBoundsContextEvenWithCustomClient(t *testing.T) {
	client := New("https://tsa.example/")
	client.HTTPClient = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > timestampRequestTimeout {
			t.Error("petición sin límite")
		}
		return nil, errors.New("fin de sonda local")
	})}
	hash := sha256.Sum256([]byte("prueba sintética"))
	_, _ = client.RequestTimestamp(context.Background(), hash[:], crypto.SHA256)
}
