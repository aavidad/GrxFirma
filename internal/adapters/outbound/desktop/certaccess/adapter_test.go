// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certaccess

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyBrowser(t *testing.T) {
	tests := map[string]string{
		"/usr/bin/firefox":                  "firefox",
		"google-chrome --profile-directory": "chrome",
		"org.chromium.Chromium":             "chromium",
		"microsoft-edge":                    "edge",
		"brave-browser":                     "brave",
		"/usr/bin/navegador-desconocido":    "",
	}
	for input, want := range tests {
		if got := classifyBrowser(input); got != want {
			t.Errorf("classifyBrowser(%q) = %q, quiere %q", input, got, want)
		}
	}
}

func TestDetectedBrowserPriorizaHintExplicito(t *testing.T) {
	managers := []Gestor{{ID: "firefox"}, {ID: "chrome"}}
	if got := detectedBrowser("google-chrome", managers); got != "chrome" {
		t.Fatalf("detectedBrowser() = %q", got)
	}
}

func TestManagerMatchesChromiumFamily(t *testing.T) {
	if !managerMatchesBrowser("chrome-stable", "chromium") {
		t.Fatal("Chrome y Chromium deben compartir el destino NSS aplicable")
	}
}

func TestRemoveSensitiveFileEliminaElTemporal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credencial.p12")
	if err := os.WriteFile(path, []byte("material sensible"), 0o600); err != nil {
		t.Fatal(err)
	}
	removeSensitiveFile(path)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el temporal sigue accesible: %v", err)
	}
}
