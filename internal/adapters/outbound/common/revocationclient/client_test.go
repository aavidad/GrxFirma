// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"
)

func TestClientFetch_RecogeOCSPYCRL(t *testing.T) {
	fixture := newValidationFixture(t)
	ocspBody := createOCSPFixture(
		t,
		fixture.ca,
		fixture.ca,
		fixture.caKey,
		fixture.leaf.SerialNumber,
		ocsp.Good,
		fixture.now.Add(-time.Minute),
		fixture.now.Add(time.Hour),
		false,
	)
	ocspRecibido := false
	ocspSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ocspRecibido = true
		if r.Method != http.MethodPost {
			t.Fatalf("metodo OCSP inesperado: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ocspBody)
	}))
	defer ocspSrv.Close()

	crlDER := createCRLFixture(
		t,
		fixture.ca,
		fixture.caKey,
		fixture.now.Add(-time.Minute),
		fixture.now.Add(time.Hour),
		nil,
	)
	crlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("metodo CRL inesperado: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(crlDER)
	}))
	defer crlSrv.Close()

	fixture.leaf.OCSPServer = []string{ocspSrv.URL}
	fixture.leaf.CRLDistributionPoints = []string{crlSrv.URL}

	client := newValidationClient(t, fixture.now)
	evidence, err := client.Fetch(context.Background(), fixture.leaf, fixture.ca)
	if err != nil {
		t.Fatalf("Fetch devolvio error inesperado: %v", err)
	}
	if !ocspRecibido {
		t.Fatal("el cliente no envio ninguna petición OCSP")
	}
	if got := len(evidence.OCSPResponses); got != 1 {
		t.Fatalf("OCSPResponses=%d, want 1", got)
	}
	if got := len(evidence.CRLs); got != 1 {
		t.Fatalf("CRLs=%d, want 1", got)
	}
}
