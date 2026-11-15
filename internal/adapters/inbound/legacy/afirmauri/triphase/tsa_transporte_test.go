// SPDX-License-Identifier: EUPL-1.2
package triphase

import (
	"context"
	"errors"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestTrifasicaNoCedeTSAHTTPAlServidorSinAjuste(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	presign := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"dat":"ZG9j","format":"XAdES","algorithm":"SHA256withRSA","extraParams":"profile=T\ntsaURL=http://tsa.example.invalid/tsr"}`
		if r.URL.Query().Get("op") != "get" {
			presign++
			body = "fallo"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	session := sesionTest("https://servidor.example/retrieve", "https://servidor.example/upload", "")
	_, err := New(client).Execute(context.Background(), session, jobTest())
	var block *excepciones.Block
	if !errors.As(err, &block) || block.ID != "tsa_http" || presign != 0 {
		t.Fatalf("TSA HTTP cedida a prefirma: err=%v requests=%d", err, presign)
	}
}

func TestTrifasicaTSAHTTPPermisoAvisoYDirectiva(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", true)
	posts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"dat":"ZG9j","format":"CAdES","algorithm":"SHA256withRSA","extraParams":"profile=T\ntsaURL=http://tsa.example.invalid/tsr"}`
		if r.URL.Query().Get("op") != "get" {
			posts++
			body = "fallo"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	session := sesionTest("https://servidor.example/retrieve", "https://servidor.example/upload", "")
	call := func() (error, []string) {
		ctx, warnings := domain.ConAvisosOperacion(context.Background())
		_, err := New(client).Execute(ctx, session, jobTest())
		return err, warnings.Claves()
	}
	err, warnings := call()
	var blocked *excepciones.Block
	if err == nil || errors.As(err, &blocked) || posts != 1 || !slices.Contains(warnings, excepciones.Warning("tsa_http")) {
		t.Fatalf("permitido: err=%v posts=%d warnings=%v", err, posts, warnings)
	}
	machinepolicy.SetForTest(t, "permitir_tsa_http", false)
	err, _ = call()
	if !errors.As(err, &blocked) || posts != 1 {
		t.Fatalf("prohibición: err=%v posts=%d", err, posts)
	}
	machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
	err, _ = call()
	if !errors.As(err, &blocked) || posts != 1 {
		t.Fatalf("directiva ilegible: err=%v posts=%d", err, posts)
	}
}

func TestLoteTrifasicoNoCedeTSAHTTPPorExtraParams(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{
		{"global", `{"algorithm":"SHA256withRSA","extraparams":"tsaURL=http://tsa.example.invalid/tsr","singlesigns":[{"id":"doc-1"}]}`},
		{"por_firma", `{"algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","extraparams":"tsaURL=http://tsa.example.invalid/tsr"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, "permitir_tsa_http")
			excepciones.SetForTest(t, "tsa_http", false)
			posts := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				posts++
				return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("error")), Header: http.Header{}}, nil
			})}
			req := afirmauri.RemoteBatchCommand{Payload: []byte(tc.payload), IsJSONBatch: true, PreSignEndpoint: "https://servidor.example/pre", PostSignEndpoint: "https://servidor.example/post"}
			ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{chain: [][]byte{validTriphaseTestCertificateDER()}})
			_, err := NewBatch(New(client)).ExecuteLegacyRemoteResult(ctx, req)
			var block *excepciones.Block
			if !errors.As(err, &block) || block.ID != "tsa_http" || posts != 0 {
				t.Fatalf("lote cedió TSA HTTP: err=%v posts=%d", err, posts)
			}
		})
	}
}
