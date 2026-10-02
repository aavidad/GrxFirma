// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type verificadorConDictamen struct {
	llamadas int
}

func (v *verificadorConDictamen) Execute(_ context.Context, cmd application.VerifyCommand) (application.VerifyResult, error) {
	v.llamadas++
	dictamen := domain.DictamenVerificacion{
		Formato:             "CAdES",
		ComprobadoEn:        time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
		Integridad:          domain.AspectoDictamen{Estado: domain.IntegridadValida},
		VinculoOriginal:     domain.AspectoDictamen{Estado: domain.VinculoAcreditado, Fuente: "resumen_contenido_firmado"},
		HuellaFirmadoSHA256: strings.Repeat("a", 64),
		Firmantes: []domain.DictamenFirmante{{
			CertificadoHuellaSHA256: strings.Repeat("b", 64),
			Cadena:                  domain.AspectoDictamen{Estado: domain.CadenaValida, Fuente: "anclas_locales"},
			Certificado:             domain.AspectoDictamen{Estado: domain.CertificadoVigente},
			Revocacion:              domain.AspectoDictamen{Estado: domain.RevocacionNoComprobada, Motivo: "sin_crl_locales"},
			SelloTiempo:             domain.AspectoDictamen{Estado: domain.SelloNoPresente},
		}},
	}.Componer()
	return application.VerifyResult{
		Verification: domain.NewVerificationSuccess("CAdES", "firma CAdES válida", nil),
		Dictamen:     &dictamen,
	}, nil
}

func TestRoutesSoloVerificacion_ExponeSoloSaludYVerify(t *testing.T) {
	verificador := &verificadorConDictamen{}
	handler := New(nil, verificador, nil).WithBearerToken("token-sintetico-de-prueba").RoutesSoloVerificacion()

	for _, ruta := range []string{"/sign", "/sign-batch", "/certificates", "/certificates/import", "/protect", "/settings", "/service/install", "/validator", "/openapi.json", "/"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, ruta, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s devolvió %d; en modo solo verificación debe ser 404", ruta, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthReq.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
	handler.ServeHTTP(rec, healthReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modo":"solo_verificacion"`) {
		t.Fatalf("/health=%d %s", rec.Code, rec.Body.String())
	}

	cuerpo := `{"name":"documento","content_base64":"` + base64.StdEncoding.EncodeToString([]byte("firma")) + `"}`
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(cuerpo)))
	if rec.Code != http.StatusUnauthorized || verificador.llamadas != 0 {
		t.Fatalf("/verify sin token=%d llamadas=%d", rec.Code, verificador.llamadas)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{"inputPath":"/etc/passwd"}`))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || verificador.llamadas != 0 {
		t.Fatalf("/verify con ruta=%d llamadas=%d", rec.Code, verificador.llamadas)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(cuerpo))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/verify=%d %s", rec.Code, rec.Body.String())
	}
	var respuesta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &respuesta); err != nil {
		t.Fatal(err)
	}
	// Compatibilidad: los campos anteriores siguen presentes.
	for _, campo := range []string{"ok", "valid", "reason", "result"} {
		if _, ok := respuesta[campo]; !ok {
			t.Fatalf("falta el campo heredado %q", campo)
		}
	}
	dictamen, ok := respuesta["dictamen"].(map[string]any)
	if !ok {
		t.Fatalf("falta dictamen: %s", rec.Body.String())
	}
	if dictamen["contrato"] != domain.ContratoDictamenVerificacion || dictamen["estado"] != "indeterminada" ||
		dictamen["motivo"] != "revocacion_no_acreditada" || dictamen["comprobadoEn"] != "2026-09-25T10:00:00Z" ||
		dictamen["certificadoHuellaSHA256"] != strings.Repeat("b", 64) {
		t.Fatalf("dictamen inesperado: %v", dictamen)
	}
	revocacion := dictamen["revocacion"].(map[string]any)
	if revocacion["estado"] != "no_comprobada" || revocacion["motivo"] != "sin_crl_locales" {
		t.Fatalf("revocación=%v", revocacion)
	}
	extensiones := dictamen["extensiones"].(map[string]any)
	if extensiones["revocacionRemota"] != "desactivada" || extensiones["selloTiempoRemoto"] != "desactivada" {
		t.Fatalf("extensiones=%v", extensiones)
	}
}

func TestRoutesSoloVerificacion_SinRedireccionesYBearerObligatorio(t *testing.T) {
	handler := New(nil, &verificadorConDictamen{}, nil).WithBearerToken("token-sintetico-de-prueba").RoutesSoloVerificacion()
	for _, path := range []string{"//verify", "/verify/", "/v1/../verify", "/%76erify"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Location") != "" {
			t.Fatalf("ruta %q: status=%d location=%q", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/health sin Bearer=%d", rec.Code)
	}
	for _, header := range []struct{ name, value string }{
		{"Authorization", "token-sintetico-de-prueba"},
		{"X-API-Token", "token-sintetico-de-prueba"},
		{"Authorization", "Basic token-sintetico-de-prueba"},
	} {
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set(header.name, header.value)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("cabecera %s=%q: status=%d", header.name, header.value, rec.Code)
		}
	}
	for _, extraHeader := range []string{"Authorization", "X-API-Token"} {
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
		req.Header.Add(extraHeader, "Bearer token-sintetico-de-prueba")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("cabecera duplicada %s: status=%d", extraHeader, rec.Code)
		}
	}
}

func TestContratoSolicitadoV2_NoCaeEnV1(t *testing.T) {
	verificador := &verificadorConDictamen{}
	adaptador := New(nil, verificador, nil).WithBearerToken("token-sintetico-de-prueba")
	handler := adaptador.RoutesSoloVerificacion()
	casos := []struct {
		ruta   string
		body   string
		status int
	}{
		{"/verify", `{"contrato_solicitado":"desconocido"}`, http.StatusBadRequest},
		{"/verify", `{"contrato_solicitado":"autofirmav2.dictamen-verificacion.v2","Contrato_Solicitado":""}`, http.StatusBadRequest},
		{"/verify", `{"content_base64":"YQ==","contrato_solicitado":"autofirmav2.dictamen-verificacion.v2"}`, http.StatusOK},
		{"/v2/verify", `{"content_base64":"YQ=="}`, http.StatusOK},
		{"/v2/verify", `{"content_base64":"YQ==","contrato_solicitado":"otro"}`, http.StatusBadRequest},
		{"/v2/verify", `{"content_base64":"YQ==","content_base64":"Yg=="}`, http.StatusBadRequest},
		{"/v2/verify", `{"content_base64":"YQ==","extra":true}`, http.StatusBadRequest},
		{"/v2/verify", `{"Content_Base64":"YQ=="}`, http.StatusBadRequest},
	}
	for _, caso := range casos {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, caso.ruta, strings.NewReader(caso.body))
		req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
		handler.ServeHTTP(rec, req)
		if rec.Code != caso.status {
			t.Fatalf("%s %s: status=%d, body=%s", caso.ruta, caso.body, rec.Code, rec.Body.String())
		}
	}
	if verificador.llamadas != 0 {
		t.Fatalf("una petición v2 acabó en el verificador v1: %d", verificador.llamadas)
	}
	adaptador.MaxBodyBytes = 32
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/verify", strings.NewReader(strings.Repeat("x", 33)))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("cuerpo grande=%d, esperado 413", rec.Code)
	}
}

func TestRechazarClavesJSONDuplicadas_EscapesYAnidacion(t *testing.T) {
	for _, jsonBody := range []string{`{"a":1,"\u0061":2}`, `{"x":{"a":1,"a":2}}`, `{"a":1} {"b":2}`} {
		if err := rechazarClavesJSONDuplicadas([]byte(jsonBody)); err == nil {
			t.Fatalf("JSON aceptado: %s", jsonBody)
		}
	}
}
