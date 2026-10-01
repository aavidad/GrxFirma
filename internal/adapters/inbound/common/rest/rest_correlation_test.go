// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/observability/correlation"
)

var errCorrelationCaptured = errors.New("caso de uso detenido tras capturar correlacion")

type restCorrelationCapture struct {
	calls     int
	reference string
	ctxDump   string
}

func (c *restCorrelationCapture) record(ctx context.Context) error {
	c.calls++
	c.ctxDump = fmt.Sprintf("%#v", ctx)
	value, ok := correlation.From(ctx)
	if !ok {
		return errors.New("caso de uso sin correlacion")
	}
	c.reference = value.Reference()
	return errCorrelationCaptured
}

type correlationSignUseCase struct{ capture *restCorrelationCapture }

func (u correlationSignUseCase) Execute(ctx context.Context, _ application.SignCommand) (application.SignResult, error) {
	return application.SignResult{}, u.capture.record(ctx)
}

type correlationBatchUseCase struct{ capture *restCorrelationCapture }

func (u correlationBatchUseCase) Execute(ctx context.Context, _ application.ProcessBatchCommand) (application.BatchResult, error) {
	return application.BatchResult{}, u.capture.record(ctx)
}

type correlationVerifyUseCase struct{ capture *restCorrelationCapture }

func (u correlationVerifyUseCase) Execute(ctx context.Context, _ application.VerifyCommand) (application.VerifyResult, error) {
	return application.VerifyResult{}, u.capture.record(ctx)
}

type correlationHashUseCase struct{ capture *restCorrelationCapture }

func (u correlationHashUseCase) Execute(ctx context.Context, _ application.CreateHashCommand) (application.CreateHashResult, error) {
	return application.CreateHashResult{}, u.capture.record(ctx)
}

type correlationProtectUseCase struct{ capture *restCorrelationCapture }

func (u correlationProtectUseCase) Execute(ctx context.Context, _ application.ProtectCommand) (application.ProtectResult, error) {
	return application.ProtectResult{}, u.capture.record(ctx)
}

type correlationUnprotectUseCase struct{ capture *restCorrelationCapture }

func (u correlationUnprotectUseCase) Execute(ctx context.Context, _ application.UnprotectCommand) (application.UnprotectResult, error) {
	return application.UnprotectResult{}, u.capture.record(ctx)
}

type correlationCatalog struct{ capture *restCorrelationCapture }

func (c correlationCatalog) List(ctx context.Context) ([]domain.CertificateRef, error) {
	err := c.capture.record(ctx)
	if errors.Is(err, errCorrelationCaptured) {
		return nil, nil
	}
	return nil, err
}

func TestRESTCorrelationMiddleware_GeneraContextoSinExponerCabeceras(t *testing.T) {
	t.Parallel()

	var captured restCorrelationCapture
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := captured.record(r.Context()); !errors.Is(err, errCorrelationCaptured) {
			t.Errorf("record(context) error = %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	restCorrelationMiddleware(next).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/validator", nil),
	)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want %d", response.Code, http.StatusNoContent)
	}
	assertOpaqueRESTCorrelation(t, captured, "")
	for _, header := range []string{"X-Request-ID", "X-Correlation-ID", "Traceparent", "Access-Control-Expose-Headers"} {
		if got := response.Header().Get(header); got != "" {
			t.Fatalf("la correlacion expuso %s=%q", header, got)
		}
	}
}

func TestRoutes_PropaganCorrelacionGeneradaEnOperacionesVisibles(t *testing.T) {
	t.Parallel()

	encodedDocument := base64.StdEncoding.EncodeToString([]byte("contenido"))
	tests := []struct {
		name     string
		endpoint string
		body     map[string]any
		build    func(*restCorrelationCapture) *Adaptador
	}{
		{
			name:     "firma legacy sin request_id",
			endpoint: "/sign",
			body: map[string]any{
				"name":           "documento.txt",
				"content_base64": encodedDocument,
				"mime_type":      "text/plain",
				"format":         "CAdES",
				"action":         "sign",
				"certificate_id": "cert-1",
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(correlationSignUseCase{capture: c}, nil, nil)
			},
		},
		{
			name:     "lote legacy sin request_id",
			endpoint: "/sign-batch",
			body: map[string]any{
				"format":         "CAdES",
				"action":         "sign",
				"certificate_id": "cert-1",
				"items": []map[string]any{{
					"name":           "documento.txt",
					"content_base64": encodedDocument,
					"mime_type":      "text/plain",
				}},
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(nil, nil, nil).WithBatchSigner(correlationBatchUseCase{capture: c})
			},
		},
		{
			name:     "verificacion",
			endpoint: "/verify",
			body: map[string]any{
				"name":           "firma.csig",
				"content_base64": encodedDocument,
				"mime_type":      "application/pkcs7-signature",
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(nil, correlationVerifyUseCase{capture: c}, nil)
			},
		},
		{
			name:     "hash",
			endpoint: "/hash",
			body: map[string]any{
				"name":           "documento.txt",
				"content_base64": encodedDocument,
				"algorithm":      "SHA-256",
				"format":         "hex",
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(nil, nil, nil).WithHashes(correlationHashUseCase{capture: c}, nil)
			},
		},
		{
			name:     "proteccion",
			endpoint: "/protect",
			body: map[string]any{
				"name":           "documento.txt",
				"content_base64": encodedDocument,
				"mime_type":      "text/plain",
				"profile":        "compat",
				"recipient_ids":  []string{"dest-1"},
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(nil, nil, nil).WithProtection(correlationProtectUseCase{capture: c}, nil, nil)
			},
		},
		{
			name:     "desproteccion",
			endpoint: "/unprotect",
			body: map[string]any{
				"name":           "documento.afp",
				"content_base64": encodedDocument,
				"mime_type":      domain.MIMETypeProtectedEnvelope,
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(nil, nil, nil).WithProtection(nil, correlationUnprotectUseCase{capture: c}, nil)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var captured restCorrelationCapture
			response := doRESTCorrelationJSON(t, test.build(&captured).Routes(), test.endpoint, test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d; want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			assertOpaqueRESTCorrelation(t, captured, "")
		})
	}
}

func TestRoutes_RequestIDValidoReemplazaCorrelacionGeneradaEnFirmaYLote(t *testing.T) {
	t.Parallel()

	const rawID = "web-operation-123"
	encodedDocument := base64.StdEncoding.EncodeToString([]byte("contenido"))
	tests := []struct {
		name     string
		endpoint string
		body     map[string]any
		build    func(*restCorrelationCapture) *Adaptador
	}{
		{
			name:     "firma",
			endpoint: "/sign",
			body: map[string]any{
				"request_id":     rawID,
				"name":           "documento.txt",
				"content_base64": encodedDocument,
				"mime_type":      "text/plain",
				"format":         "CAdES",
				"action":         "sign",
				"certificate_id": "cert-1",
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(correlationSignUseCase{capture: c}, nil, nil)
			},
		},
		{
			name:     "lote",
			endpoint: "/sign-batch",
			body: map[string]any{
				"request_id":     rawID,
				"format":         "CAdES",
				"action":         "sign",
				"certificate_id": "cert-1",
				"items": []map[string]any{{
					"name":           "documento.txt",
					"content_base64": encodedDocument,
					"mime_type":      "text/plain",
				}},
			},
			build: func(c *restCorrelationCapture) *Adaptador {
				return New(nil, nil, nil).WithBatchSigner(correlationBatchUseCase{capture: c})
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var captured restCorrelationCapture
			response := doRESTCorrelationJSON(t, test.build(&captured).Routes(), test.endpoint, test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d; want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			assertOpaqueRESTCorrelation(t, captured, rawID)
		})
	}
}

func TestRoutes_RechazanRequestIDInvalidoAntesDelCasoDeUso(t *testing.T) {
	t.Parallel()

	invalidIDs := []string{
		"",
		"request\ncontrol",
		"request-ñ",
		strings.Repeat("x", 129),
	}
	encodedDocument := base64.StdEncoding.EncodeToString([]byte("contenido"))
	for _, rawID := range invalidIDs {
		rawID := rawID
		t.Run(fmt.Sprintf("%x", []byte(rawID)), func(t *testing.T) {
			t.Parallel()

			for _, endpoint := range []string{"/sign", "/sign-batch"} {
				var captured restCorrelationCapture
				var adapter *Adaptador
				var body map[string]any
				switch endpoint {
				case "/sign":
					adapter = New(correlationSignUseCase{capture: &captured}, nil, nil)
					body = map[string]any{
						"request_id":     rawID,
						"name":           "documento.txt",
						"content_base64": encodedDocument,
						"mime_type":      "text/plain",
						"format":         "CAdES",
						"certificate_id": "cert-1",
					}
				case "/sign-batch":
					adapter = New(nil, nil, nil).WithBatchSigner(correlationBatchUseCase{capture: &captured})
					body = map[string]any{
						"request_id": rawID,
						"items": []map[string]any{{
							"name":           "documento.txt",
							"content_base64": encodedDocument,
							"mime_type":      "text/plain",
						}},
					}
				}

				response := doRESTCorrelationJSON(t, adapter.Routes(), endpoint, body)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("%s status = %d; want %d; body=%s", endpoint, response.Code, http.StatusBadRequest, response.Body.String())
				}
				if captured.calls != 0 {
					t.Fatalf("%s ejecuto el caso de uso %d veces", endpoint, captured.calls)
				}
				if rawID != "" && strings.Contains(response.Body.String(), rawID) {
					t.Fatalf("%s reflejo el request_id rechazado", endpoint)
				}
			}
		})
	}
}

func TestRoutes_DiagnosticsPropagaCorrelacionGenerada(t *testing.T) {
	t.Parallel()

	var captured restCorrelationCapture
	adapter := New(nil, nil, nil)
	adapter.Catalogo = correlationCatalog{capture: &captured}

	response := httptest.NewRecorder()
	adapter.Routes().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/diagnostics/report", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	assertOpaqueRESTCorrelation(t, captured, "")
	if strings.Contains(response.Body.String(), captured.reference) {
		t.Fatal("diagnostics expuso la referencia interna en su respuesta")
	}
}

func assertOpaqueRESTCorrelation(t *testing.T, captured restCorrelationCapture, rawID string) {
	t.Helper()
	if captured.calls != 1 {
		t.Fatalf("llamadas capturadas = %d; want 1", captured.calls)
	}
	if !strings.HasPrefix(captured.reference, "corr-sha256-") {
		t.Fatalf("referencia = %q; falta correlacion SHA-256 opaca", captured.reference)
	}
	if rawID != "" {
		if strings.Contains(captured.reference, rawID) {
			t.Fatal("la referencia contiene request_id bruto")
		}
		if strings.Contains(captured.ctxDump, rawID) {
			t.Fatal("context.Context contiene request_id bruto")
		}
	}
}

func doRESTCorrelationJSON(t *testing.T, handler http.Handler, endpoint string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
