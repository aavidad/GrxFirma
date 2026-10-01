// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package padesviewer

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAbrir_ProtegeVisorYDescargaConToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := New()
	url, err := srv.Abrir(ctx, []byte("%PDF-test"), "firmado.pdf")
	if err != nil {
		t.Fatalf("Abrir() error = %v", err)
	}
	defer srv.CerrarTodos()

	if !strings.Contains(url, "token=") {
		t.Fatalf("url sin token: %q", url)
	}

	base := strings.Split(url, "/?")[0]
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET sin token: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET sin token status = %d, want 403", resp.StatusCode)
	}

	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET con token: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll visor: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET con token status = %d cuerpo=%s", resp.StatusCode, string(body))
	}
	for header, esperado := range map[string]string{
		"Cache-Control":           "no-store, private, max-age=0",
		"Content-Security-Policy": "default-src 'none'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := resp.Header.Get(header); !strings.Contains(got, esperado) {
			t.Errorf("%s = %q, want contiene %q", header, got, esperado)
		}
	}
	if !strings.Contains(string(body), "/pades/guardar?token=") {
		t.Fatalf("visor no incluye descarga tokenizada: %s", string(body))
	}

	descarga := strings.Replace(url, "/?token=", "/pades/guardar?token=", 1)
	resp, err = http.Get(descarga)
	if err != nil {
		t.Fatalf("GET descarga con token: %v", err)
	}
	body, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll descarga: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("descarga status = %d cuerpo=%s", resp.StatusCode, string(body))
	}
	if string(body) != "%PDF-test" {
		t.Fatalf("descarga = %q", string(body))
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("NewRequest POST: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST visor: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST status=%d Allow=%q, want 405 y GET, HEAD", resp.StatusCode, resp.Header.Get("Allow"))
	}

	resp, err = http.Get(base + "/ruta-inexistente?" + strings.SplitN(url, "?", 2)[1])
	if err != nil {
		t.Fatalf("GET ruta inexistente: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ruta inexistente status=%d, want 404", resp.StatusCode)
	}
}
