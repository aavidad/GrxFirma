// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// B-08: una web ajena no puede agotar los retos del login por certificado.
func TestLoginCertificado_WebAjenaNoAgotaLosRetos(t *testing.T) {
	h := New(nil, nil, nil).WithCertificateAuth(strings.Repeat("a", sha256.Size*2), time.Minute).Routes()

	for i := 0; i < maxPendingRESTChallenges+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "https://127.0.0.1:63118/auth/challenge", nil)
		req.Header.Set("Origin", "https://atacante.example")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("reto desde origen ajeno = %d; se esperaba 403", rr.Code)
		}
	}
	for _, origen := range []string{"", "https://127.0.0.1:63118"} {
		req := httptest.NewRequest(http.MethodPost, "https://127.0.0.1:63118/auth/challenge", nil)
		if origen != "" {
			req.Header.Set("Origin", origen)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("cliente legítimo (Origin=%q) = %d %s", origen, rr.Code, rr.Body.String())
		}
	}
}

func TestLoginCertificado_LimitaElCuerpo(t *testing.T) {
	h := New(nil, nil, nil).WithCertificateAuth(strings.Repeat("a", sha256.Size*2), time.Minute).Routes()
	for _, ruta := range []string{"/auth/verify", "/autenticacion/verificar", "/auth/challenge"} {
		cuerpo := bytes.Repeat([]byte("x"), 1<<20)
		req := httptest.NewRequest(http.MethodPost, ruta, bytes.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s con 1 MB = %d; se esperaba 413", ruta, rr.Code)
		}
	}
}
