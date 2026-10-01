// Copyright (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: MIT

package sign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
)

func timestampTestHandler(t *testing.T, mutate func(*timestamp.Timestamp)) http.Handler {
	t.Helper()
	cert, key := loadCertificateAndKey(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/timestamp-query" {
			t.Error("incorrect timestamp request method or content type")
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		if err != nil || len(body) > 65536 {
			http.Error(w, "invalid request", 400)
			return
		}
		req, err := timestamp.ParseRequest(body)
		if err != nil {
			t.Error(err)
			http.Error(w, "invalid request", 400)
			return
		}
		response := &timestamp.Timestamp{
			HashAlgorithm: req.HashAlgorithm, HashedMessage: req.HashedMessage,
			Nonce: req.Nonce, Time: time.Now().UTC(), AddTSACertificate: true,
			Policy: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1},
		}
		if mutate != nil {
			mutate(response)
		}
		der, err := response.CreateResponseWithOpts(cert, key, crypto.SHA256)
		if err != nil {
			t.Error(err)
			http.Error(w, "cannot respond", 500)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(der)
	})
}

func TestTimestampHTTPResponseBinding(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*timestamp.Timestamp)
		want   string
	}{
		{"correct", nil, ""},
		{"imprint", func(ts *timestamp.Timestamp) { ts.HashedMessage[0] ^= 1 }, "messageImprint"},
		{"algorithm", func(ts *timestamp.Timestamp) { ts.HashAlgorithm = crypto.SHA512 }, "messageImprint"},
		{"nonce", func(ts *timestamp.Timestamp) { ts.Nonce.Add(ts.Nonce, big.NewInt(1)) }, "nonce"},
		{"missing nonce", func(ts *timestamp.Timestamp) { ts.Nonce = nil }, "nonce"},
		{"missing cert", func(ts *timestamp.Timestamp) { ts.AddTSACertificate = false }, "certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(timestampTestHandler(t, tc.mutate))
			defer server.Close()
			body, err := requestTimestamp(TSA{URL: server.URL}, []byte("synthetic PDF signature"), crypto.SHA256)
			if tc.want == "" {
				if err != nil || len(body) == 0 {
					t.Fatalf("valid response rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}

func TestTimestampHTTPLimitsAndSanitizedErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    []byte
		chunked bool
		want    string
	}{
		{"sized", 200, bytes.Repeat([]byte{0}, maxTimestampResponseBytes+1), false, "size limit"},
		{"chunked", 200, bytes.Repeat([]byte{0}, maxTimestampResponseBytes+1), true, "size limit"},
		{"http error", 500, []byte("private-response-marker"), false, "status 500"},
		{"invalid token", 200, []byte("private-response-marker"), false, "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.chunked {
					w.Header().Set("Content-Length", big.NewInt(int64(len(tc.body))).String())
				}
				w.WriteHeader(tc.status)
				if tc.chunked {
					w.(http.Flusher).Flush()
				}
				_, _ = w.Write(tc.body)
			}))
			defer server.Close()
			_, err := requestTimestamp(TSA{URL: server.URL}, []byte("synthetic signature"), crypto.SHA256)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-response-marker") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTimestampHTTPCancellationDuringResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()
	_, err := requestTimestamp(TSA{URL: server.URL, Context: ctx}, []byte("synthetic signature"), crypto.SHA256)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestTimestampHTTPRedirectsAndConfiguredTransport(t *testing.T) {
	var unwanted atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { unwanted.Add(1) }))
	defer destination.Close()
	valid := timestampTestHandler(t, nil)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", 307)
		case "/downgrade":
			http.Redirect(w, r, destination.URL, 307)
		case "/get":
			http.Redirect(w, r, "/final", 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 307)
		default:
			valid.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	configured := server.Client()
	for _, path := range []string{"/start", "/downgrade", "/get", "/loop"} {
		_, err := requestTimestamp(TSA{URL: server.URL + path, HTTPClient: configured}, []byte("synthetic signature"), crypto.SHA256)
		if (err == nil) != (path == "/start") {
			t.Fatalf("path %s: %v", path, err)
		}
	}
	if unwanted.Load() != 0 {
		t.Fatal("timestamp escaped initial origin")
	}
	if configured.Timeout != 0 || configured.CheckRedirect != nil {
		t.Fatal("shared HTTP client modified")
	}
}

func TestTimestampHTTPBoundedDefault(t *testing.T) {
	for _, value := range []time.Duration{0, time.Hour, -time.Second} {
		configured := &http.Client{Timeout: value}
		if timestampHTTPClient(configured).Timeout != timestampRequestTimeout || configured.Timeout != value {
			t.Fatal("missing timeout or mutated configuration")
		}
	}
	configured := &http.Client{Timeout: time.Millisecond}
	if timestampHTTPClient(configured).Timeout != configured.Timeout {
		t.Fatal("shorter timeout not preserved")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := requestTimestamp(TSA{URL: "https://tsa.example", Context: ctx}, sha256.New().Sum(nil), crypto.SHA256)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
