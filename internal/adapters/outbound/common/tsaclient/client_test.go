// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tsaclient

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/digitorus/timestamp"
	"grxfirma/internal/testsupport/tsatest"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClient_RequestTimestamp_Mock(t *testing.T) {
	responder := tsatest.NewResponder(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("metodo HTTP inesperado: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/timestamp-query" {
			t.Errorf("Content-Type inesperado: %s", ct)
		}
		responder.ServeHTTP(w, r)
	}))
	defer srv.Close()

	client := New(srv.URL)
	hash := sha256.Sum256([]byte("datos de prueba"))
	tst, err := client.RequestTimestamp(context.Background(), hash[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if len(tst) == 0 {
		t.Fatal("el TST retornado esta vacio")
	}
}

func TestClient_RequestTimestamp_RechazaTokenNoLigadoALaPeticion(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*timestamp.Timestamp)
		want   string
	}{
		{
			name: "message imprint distinto",
			mutate: func(response *timestamp.Timestamp) {
				response.HashedMessage[0] ^= 0xff
			},
			want: "messageImprint",
		},
		{
			name: "nonce distinto",
			mutate: func(response *timestamp.Timestamp) {
				response.Nonce.Add(response.Nonce, bigOne)
			},
			want: "nonce",
		},
		{
			name: "nonce ausente",
			mutate: func(response *timestamp.Timestamp) {
				response.Nonce = nil
			},
			want: "nonce",
		},
		{
			name: "algoritmo distinto",
			mutate: func(response *timestamp.Timestamp) {
				response.HashAlgorithm = crypto.SHA512
			},
			want: "algoritmo distinto",
		},
		{
			name: "certificado TSA ausente",
			mutate: func(response *timestamp.Timestamp) {
				response.AddTSACertificate = false
			},
			want: "sin certificado TSA",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responder := tsatest.NewResponder(t)
			srv := mutatingTimestampServer(t, responder, test.mutate)
			defer srv.Close()

			hash := sha256.Sum256([]byte("datos de prueba"))
			_, err := New(srv.URL).RequestTimestamp(
				context.Background(),
				hash[:],
				crypto.SHA256,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RequestTimestamp() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestClient_RequestTimestamp_ErrorHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(srv.URL)
	hash := sha256.Sum256([]byte("datos"))
	_, err := client.RequestTimestamp(context.Background(), hash[:], crypto.SHA256)
	if err == nil {
		t.Fatal("se esperaba error por HTTP 500")
	}
}

func TestClient_RequestTimestamp_ContextoCancelado(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := New(srv.URL)
	hash := sha256.Sum256([]byte("datos"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.RequestTimestamp(ctx, hash[:], crypto.SHA256)
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}

func TestClient_RequestTimestamp_RechazaLongitudDeHashInvalida(t *testing.T) {
	client := New("https://tsa.example")
	if _, err := client.RequestTimestamp(
		context.Background(),
		[]byte("no-es-un-sha256"),
		crypto.SHA256,
	); err == nil || !strings.Contains(err.Error(), "longitud de hash invalida") {
		t.Fatalf("RequestTimestamp() error = %v, want longitud invalida", err)
	}
}

func TestClient_RequestTimestamp_RechazaRespuestaExcesiva(t *testing.T) {
	t.Parallel()

	client := New("https://tsa.example")
	client.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: -1,
				Body: io.NopCloser(bytes.NewReader(
					bytes.Repeat([]byte{0x30}, maxTimestampResponseBytes+1),
				)),
				Header: make(http.Header),
			}, nil
		}),
	}
	hash := sha256.Sum256([]byte("datos"))
	if _, err := client.RequestTimestamp(context.Background(), hash[:], crypto.SHA256); err == nil {
		t.Fatal("RequestTimestamp() acepto una respuesta por encima del limite")
	}
}

var bigOne = big.NewInt(1)

func mutatingTimestampServer(
	t *testing.T,
	responder *tsatest.Responder,
	mutate func(*timestamp.Timestamp),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestDER, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		responseDER, err := responder.ResponseFor(requestDER, mutate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(responseDER)
	}))
}
