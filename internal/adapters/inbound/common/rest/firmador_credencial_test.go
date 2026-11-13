// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/domain"
)

// B-02: el firmador web debe mandar la credencial que el usuario introduce,
// igual que la consola y el validador, y nunca por la URL.
func TestFirmadorWeb_ServidorRealConCredencial_RecorreElFlujo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const token = "credencial-firmador-prueba"
	adaptador := rest.New(signUseCaseMock{}, nil, nil).
		WithBearerToken(token).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-1",
			Subject:     "CN=Firma",
			Issuer:      "CN=CA",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp-1",
		}}}, nil)
	tlsDir := t.TempDir()
	srv, err := rest.StartTLSServer(ctx, "127.0.0.1:0", adaptador.Routes(), tlsDir)
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	rootPEM, err := os.ReadFile(rest.ManagedLocalhostCAFile(tlsDir))
	if err != nil {
		t.Fatalf("CA local: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		t.Fatal("CA local ilegible")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"}}}
	base := "https://" + srv.Addr

	do := func(method, path string, body []byte, auth bool) int {
		t.Helper()
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, _ := http.NewRequest(method, base+path, rd)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	for _, ruta := range []string{"/firmador", "/signer"} {
		resp, err := client.Get(base + ruta)
		if err != nil {
			t.Fatalf("GET %s: %v", ruta, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", ruta, resp.StatusCode)
		}
		page := string(raw)
		if !regexp.MustCompile(`<input id="bearerToken" type="password"`).MatchString(page) {
			t.Fatalf("%s no ofrece campo de credencial de tipo password", ruta)
		}
		if strings.Contains(page, `localStorage.setItem("bearerToken"`) || strings.Contains(page, "bearerToken: ui.bearerToken") {
			t.Fatalf("%s persiste la credencial", ruta)
		}
		// apiFetch tiene que añadir la cabecera a todas las llamadas.
		fn := regexp.MustCompile(`(?s)async function apiFetch\(path, options\) \{.*?await fetch\(path, opts\)`).FindString(page)
		if fn == "" || !strings.Contains(fn, "authHeaders()") || !strings.Contains(page, `Authorization: "Bearer " + token`) {
			t.Fatalf("%s: apiFetch no añade Authorization", ruta)
		}
		if regexp.MustCompile(`[?&](token|bearer|access_token)=`).MatchString(page) {
			t.Fatalf("%s pone la credencial en una URL", ruta)
		}
	}

	// Recorre las rutas que llama la página: sin credencial 401, con ella no.
	resp, _ := client.Get(base + "/firmador")
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	llamadas := regexp.MustCompile(`apiFetch\("([^"]+)"`).FindAllStringSubmatch(string(raw), -1)
	if len(llamadas) < 5 {
		t.Fatalf("no se encontraron las llamadas del firmador: %v", llamadas)
	}
	for _, m := range llamadas {
		ruta := m[1]
		metodo, cuerpo := http.MethodGet, []byte(nil)
		switch ruta {
		case "/select-certificate", "/verify", "/sign", "/sign-batch":
			metodo, cuerpo = http.MethodPost, []byte(`{}`)
		}
		if ruta == "/health" {
			continue
		}
		if got := do(metodo, ruta, cuerpo, false); got != http.StatusUnauthorized {
			t.Fatalf("%s %s sin credencial = %d; se esperaba 401", metodo, ruta, got)
		}
		if got := do(metodo, ruta, cuerpo, true); got == http.StatusUnauthorized {
			t.Fatalf("%s %s con credencial sigue dando 401", metodo, ruta)
		}
	}

	if got := do(http.MethodGet, "/certificates?check=1", nil, true); got != http.StatusOK {
		t.Fatalf("/certificates con credencial = %d", got)
	}
	firma, _ := json.Marshal(map[string]any{
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
		"certificate_id": "cert-1",
	})
	if got := do(http.MethodPost, "/sign", firma, true); got != http.StatusOK {
		t.Fatalf("/sign con credencial = %d", got)
	}
}
