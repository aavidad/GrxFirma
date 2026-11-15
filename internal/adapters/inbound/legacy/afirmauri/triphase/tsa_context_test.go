// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestTrifasicaTSAHTTPDeRetrieveE2EAnotaAvisoAntesDePrefirma(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", true)
	response, err := json.Marshal(map[string]string{
		"dat": "ZG9j", "format": "XAdES", "algorithm": "SHA256withRSA",
		"extraParams": "profile=baseline\ntsaURL=http://127.0.0.1:18222/tsa",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := string(response)
		if req.URL.Query().Get("op") != "get" {
			body = "error de prefirma"
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	session := sesionTest("https://servidor.example/retrieve", "https://servidor.example/upload", "")
	_, _ = New(client).Execute(ctx, session, jobTest())
	if !slices.Contains(warnings.Claves(), excepciones.Warning("tsa_http")) {
		t.Fatalf("la TSA HTTP de extraParams no llegó al resultado: %v", warnings.Claves())
	}
}
