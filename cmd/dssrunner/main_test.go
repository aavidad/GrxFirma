// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckResponse_TotalPassedEstricto(t *testing.T) {
	t.Parallel()

	resp := validDSSResponse("PAdES-BASELINE-B", "TOTAL_PASSED", "")

	summary, err := checkResponse("pades", dssResultTotalPassed, resp)
	if err != nil {
		t.Fatalf("checkResponse() error = %v", err)
	}
	if !strings.Contains(summary, "DSS TOTAL_PASSED") ||
		!strings.Contains(summary, "PAdES-BASELINE-B") {
		t.Fatalf("summary inesperado: %s", summary)
	}
}

func TestCheckResponse_IndeterminateNoPasaConformidadEstricta(t *testing.T) {
	t.Parallel()

	resp := validDSSResponse(
		"PAdES-BASELINE-B",
		"INDETERMINATE",
		"NO_CERTIFICATE_CHAIN_FOUND",
	)

	_, err := checkResponse("pades", dssResultTotalPassed, resp)
	if err == nil || !strings.Contains(err.Error(), "no alcanzó TOTAL_PASSED") {
		t.Fatalf("checkResponse() error = %v, want fallo de conformidad", err)
	}
}

func TestCheckResponse_IndeterminateSoloPasaExpectativaExplicitaDeIntegridad(t *testing.T) {
	t.Parallel()

	resp := validDSSResponse(
		"PAdES-BASELINE-B",
		"INDETERMINATE",
		"NO_CERTIFICATE_CHAIN_FOUND",
	)

	summary, err := checkResponse("pades", dssResultIntegrityFormatRecognized, resp)
	if err != nil {
		t.Fatalf("checkResponse() error = %v", err)
	}
	if !strings.Contains(summary, "DSS INTEGRITY_FORMAT_RECOGNIZED") ||
		!strings.Contains(summary, "subindicacion=NO_CERTIFICATE_CHAIN_FOUND") {
		t.Fatalf("summary inesperado: %s", summary)
	}
}

func TestCheckResponse_InvalidNoPasaNiModoDiagnostico(t *testing.T) {
	t.Parallel()

	resp := validDSSResponse("PAdES-BASELINE-B", "INVALID", "HASH_FAILURE")

	_, err := checkResponse("pades", dssResultIntegrityFormatRecognized, resp)
	if err == nil || !strings.Contains(err.Error(), "no confirmó integridad y formato") {
		t.Fatalf("checkResponse() error = %v, want rechazo de INVALID", err)
	}
}

func TestClassifyDSSResult_MapeaIndicationYSubIndication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		indication    string
		subIndication string
		want          dssResult
		wantErr       string
	}{
		{
			name:       "conformidad total",
			indication: "TOTAL_PASSED",
			want:       dssResultTotalPassed,
		},
		{
			name:          "cadena ausente queda como integridad reconocida",
			indication:    "INDETERMINATE",
			subIndication: "NO_CERTIFICATE_CHAIN_FOUND",
			want:          dssResultIntegrityFormatRecognized,
		},
		{
			name:          "revocacion indeterminada no se eleva a conformidad",
			indication:    "INDETERMINATE",
			subIndication: "REVOKED_NO_POE",
			want:          dssResultIntegrityFormatRecognized,
		},
		{
			name:          "invalid",
			indication:    "INVALID",
			subIndication: "HASH_FAILURE",
			want:          dssResultInvalid,
		},
		{
			name:          "total failed",
			indication:    "TOTAL_FAILED",
			subIndication: "SIG_CRYPTO_FAILURE",
			want:          dssResultInvalid,
		},
		{
			name:          "fallo de hash indeterminado no confirma integridad",
			indication:    "INDETERMINATE",
			subIndication: "HASH_FAILURE",
			want:          dssResultInvalid,
		},
		{
			name:          "subindicacion indeterminada desconocida falla cerrado",
			indication:    "INDETERMINATE",
			subIndication: "FUTURE_DSS_VALUE",
			wantErr:       "SubIndication no reconocida",
		},
		{
			name:       "indeterminate sin causa falla cerrado",
			indication: "INDETERMINATE",
			wantErr:    "sin SubIndication",
		},
		{
			name:       "passed antiguo no equivale a total passed",
			indication: "PASSED",
			wantErr:    "Indication no reconocida",
		},
		{
			name:          "total passed incoherente con subindicacion",
			indication:    "TOTAL_PASSED",
			subIndication: "NO_CERTIFICATE_CHAIN_FOUND",
			wantErr:       "respuesta DSS incoherente",
		},
		{
			name:    "indication ausente",
			wantErr: "Indication vacía",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := classifyDSSResult(tt.indication, tt.subIndication)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("classifyDSSResult() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyDSSResult() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("classifyDSSResult() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseExpectedResult_ExigeValorCanonicoInequivoco(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "total_passed", "TOTAL-PASSED", "PASSED"} {
		if _, err := parseExpectedResult(raw); err == nil {
			t.Errorf("parseExpectedResult(%q) debía fallar", raw)
		}
	}
	for _, raw := range []string{expectedTotalPassed, expectedIntegrityFormatRecognized} {
		if _, err := parseExpectedResult(raw); err != nil {
			t.Errorf("parseExpectedResult(%q) error = %v", raw, err)
		}
	}
}

func TestCheckResponse_FallaSiFormatoNoCoincide(t *testing.T) {
	t.Parallel()

	resp := validDSSResponse("CAdES-BASELINE-B", "TOTAL_PASSED", "")

	if _, err := checkResponse("xades", dssResultTotalPassed, resp); err == nil {
		t.Fatal("se esperaba error por formato distinto")
	}
}

func TestRun_UsaEndpointYDevuelveResumen(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("método inesperado: %s", r.Method)
		}
		var req validateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.SignedDocument.Name != "signed.bin" {
			t.Fatalf("signedDocument.Name inesperado: %s", req.SignedDocument.Name)
		}
		resp := validDSSResponse("CAdES-BASELINE-B", "TOTAL_PASSED", "")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	dir := t.TempDir()
	signedPath := filepath.Join(dir, "signed.bin")
	if err := os.WriteFile(signedPath, []byte("firma"), 0o600); err != nil {
		t.Fatalf("WriteFile signed: %v", err)
	}
	originalPath := filepath.Join(dir, "origen.txt")
	if err := os.WriteFile(originalPath, []byte("origen"), 0o600); err != nil {
		t.Fatalf("WriteFile original: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--format", "cades",
		"--signed", signedPath,
		"--original", originalPath,
		"--endpoint", server.URL,
		"--expect", expectedTotalPassed,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "DSS TOTAL_PASSED") {
		t.Fatalf("stdout inesperado: %s", stdout.String())
	}
}

func TestRun_ExigeResultadoEsperado(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--format", "cades", "--signed", "signed.bin"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--expect") {
		t.Fatalf("stderr inesperado: %s", stderr.String())
	}
}

func TestValidateDSSEndpoint_AplicaPoliticaDeTransporte(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "https remoto", endpoint: "https://dss.example.test/validate"},
		{name: "http localhost", endpoint: "http://localhost:8080/validate"},
		{name: "http ipv4 loopback", endpoint: "http://127.0.0.1:8080/validate"},
		{name: "http ipv6 loopback", endpoint: "http://[::1]:8080/validate"},
		{name: "http remoto", endpoint: "http://dss.example.test/validate", wantErr: true},
		{name: "credenciales embebidas", endpoint: "https://user:pass@dss.example.test/validate", wantErr: true},
		{name: "fragmento", endpoint: "https://dss.example.test/validate#resultado", wantErr: true},
		{name: "esquema no HTTP", endpoint: "file:///etc/passwd", wantErr: true},
		{name: "sin host", endpoint: "https:///validate", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateDSSEndpoint(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDSSEndpoint(%q) error = %v, wantErr %t", tt.endpoint, err, tt.wantErr)
			}
		})
	}
}

func TestValidate_AdmiteRedireccionDelMismoOrigen(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/result", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if _, err := validate(server.URL+"/start", validateRequest{}); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
}

func TestValidate_RechazaRedireccionAOtroOrigen(t *testing.T) {
	t.Parallel()

	var destinationHits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationHits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer destination.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	if _, err := validate(source.URL, validateRequest{}); err == nil {
		t.Fatal("validate() debe rechazar una redirección a otro origen")
	}
	if got := destinationHits.Load(); got != 0 {
		t.Fatalf("el origen no confiable recibió %d peticiones, want 0", got)
	}
}

func TestValidate_RechazaRespuestaDemasiadoGrande(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxDSSResponseBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := validate(server.URL, validateRequest{})
	if err == nil || !strings.Contains(err.Error(), "demasiado grande") {
		t.Fatalf("validate() error = %v, want respuesta demasiado grande", err)
	}
}

func TestSameDSSOrigin_RechazaCambioDeHostPuertoOEsquema(t *testing.T) {
	t.Parallel()

	base, err := url.Parse("https://dss.example.test/validate")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "https://dss.example.test/other", want: true},
		{raw: "https://DSS.EXAMPLE.TEST:443/other", want: true},
		{raw: "https://other.example.test/validate"},
		{raw: "https://dss.example.test:8443/validate"},
		{raw: "http://dss.example.test/validate"},
	}
	for _, tt := range tests {
		candidate, parseErr := url.Parse(tt.raw)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if got := sameDSSOrigin(base, candidate); got != tt.want {
			t.Errorf("sameDSSOrigin(%q) = %t, want %t", tt.raw, got, tt.want)
		}
	}
}

func validDSSResponse(format, indication, subIndication string) dssResponse {
	resp := dssResponse{}
	diagnostic := dssDiagnosticSignature{SignatureFormat: format}
	diagnostic.BasicSignature.SignatureIntact = true
	diagnostic.BasicSignature.SignatureValid = true
	resp.DiagnosticData.Signature = []dssDiagnosticSignature{diagnostic}

	simple := dssSimpleReportSignature{}
	simple.Signature.Indication = indication
	simple.Signature.SubIndication = subIndication
	resp.SimpleReport.Signatures = []dssSimpleReportSignature{simple}
	return resp
}
