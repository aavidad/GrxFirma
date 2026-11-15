// SPDX-License-Identifier: EUPL-1.2
package triphase

import (
	"context"
	"encoding/base64"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"io"
	"net/http"
	"strings"
	"testing"
)

func contextoHTTPTrifasicaConRecibo(t *testing.T) (context.Context, *domain.AvisosOperacion) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_trifasica_http")
	dir := t.TempDir()
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), dir, excepciones.ActivationRequest{ID: "trifasica_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	return domain.ConAvisosOperacion(ctx)
}

func TestV7HTTPConReciboVigenteSubeYRecuperaConAviso(t *testing.T) {
	ctx, _ := contextoHTTPTrifasicaConRecibo(t)
	requests := 0
	e := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		body := "OK"
		if r.Form.Get("op") == "get" {
			body = string(retrieveResponseJSON(base64.StdEncoding.EncodeToString([]byte("documento"))))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	session := sesionTest("http://portal.example/rt", "http://portal.example/st", "")
	for _, operation := range []string{"subir", "recuperar"} {
		opCtx, warnings := domain.ConAvisosOperacion(ctx)
		if operation == "subir" {
			if err := e.UploadCertificate(opCtx, session, []byte("cert-der"), nil); err != nil {
				t.Fatal(err)
			}
		} else {
			if data, err := e.RetrieveRaw(opCtx, session); err != nil || len(data) == 0 {
				t.Fatalf("recuperación: %q %v", data, err)
			}
		}
		assertHTTPTrifasicaWarning(t, warnings)
	}
	if requests != 2 {
		t.Fatalf("peticiones=%d", requests)
	}
}
