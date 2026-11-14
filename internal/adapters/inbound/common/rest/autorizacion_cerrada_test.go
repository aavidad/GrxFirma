// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/common/rest"
)

// B-04: una composición que olvide configurar credencial no publica la API.
func TestAuthorize_SinCredencialConfiguradaFallaCerrado(t *testing.T) {
	h := rest.New(signUseCaseMock{}, nil, nil).Routes()

	cuerpo := `{"name":"doc.txt","content_base64":"YWJj","format":"CAdES","action":"sign","certificate_id":"cert-1"}`
	for _, tc := range []struct{ metodo, ruta, ct string }{
		{http.MethodPost, "/sign", "application/json"},
		{http.MethodPost, "/sign", "text/plain"},
		{http.MethodGet, "/settings", ""},
		{http.MethodGet, "/certificates", ""},
	} {
		req := httptest.NewRequest(tc.metodo, tc.ruta, strings.NewReader(cuerpo))
		if tc.ct != "" {
			req.Header.Set("Content-Type", tc.ct)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s (%s) sin credencial configurada = %d; se esperaba 503", tc.metodo, tc.ruta, tc.ct, rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/health = %d", rr.Code)
	}
}
