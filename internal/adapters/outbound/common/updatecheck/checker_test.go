// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheck_AceptaRespuestaGitHubLatest(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://github.com/aavidad/GrxFirma/releases/tag/v1.2.3"}`))
	}))
	defer srv.Close()

	result, ok, err := Check(context.Background(), "v1.2.2", srv.URL)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() debe detectar una version mas nueva")
	}
	if result.Version != "v1.2.3" {
		t.Fatalf("result.Version = %q, want v1.2.3", result.Version)
	}
}

func TestCheck_AceptaManifestPropio(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"2.0.0","url":"https://example.invalid/download"}`))
	}))
	defer srv.Close()

	result, ok, err := Check(context.Background(), "1.9.0", srv.URL)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() debe detectar una version mas nueva")
	}
	if result.URL != "https://example.invalid/download" {
		t.Fatalf("result.URL = %q, want https://example.invalid/download", result.URL)
	}
}

func TestCheck_NoMarcaMismaVersion(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://github.com/aavidad/GrxFirma/releases/tag/v1.2.3"}`))
	}))
	defer srv.Close()

	_, ok, err := Check(context.Background(), "v1.2.3", srv.URL)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if ok {
		t.Fatal("Check() no debe marcar una actualizacion cuando la version es la misma")
	}
}

func TestCheck_VersionDevNoGeneraFalsoAviso(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0","html_url":"https://github.com/aavidad/GrxFirma/releases/tag/v1.0.0"}`))
	}))
	defer srv.Close()

	_, ok, err := Check(context.Background(), "dev", srv.URL)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if ok {
		t.Fatal("Check() no debe afirmar que un build dev es anterior a una release estable")
	}
}

func TestCheck_ConvierteRepoGitHubEnAPI(t *testing.T) {
	t.Parallel()

	owner, repo, ok := parseGitHubRepo("https://github.com/aavidad/GrxFirma")
	if !ok {
		t.Fatal("ParseGitHubRepoForTest() debe reconocer el repo GitHub")
	}
	if owner != "aavidad" || repo != "GrxFirma" {
		t.Fatalf("owner/repo = %s/%s, want aavidad/GrxFirma", owner, repo)
	}
}
