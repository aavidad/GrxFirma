// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package truststore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/domain"
)

func TestOriginMatches_ExigeEsquemaYPuerto(t *testing.T) {
	t.Parallel()

	casos := []struct {
		origin, patron string
		want           bool
	}{
		{"https://sede.ejemplo.gob.es", "*.gob.es", true},
		{"https://sede.ejemplo.gob.es:8443", "*.gob.es", true},
		{"http://sede.ejemplo.gob.es", "*.gob.es", false},
		{"wss://sede.ejemplo.gob.es", "*.gob.es", false},
		{"https://gob.es.malicioso.com", "*.gob.es", false},
		{"https://sede.ejemplo.es:8443", "sede.ejemplo.es:8443", true},
		{"https://sede.ejemplo.es", "sede.ejemplo.es:8443", false},
		{"https://127.0.0.1:63118", "https://127.0.0.1:63118", true},
		{"https://127.0.0.1:8080", "https://127.0.0.1:63118", false},
		{"http://127.0.0.1:63118", "https://127.0.0.1:63118", false},
		{"http://localhost:3000", "http://localhost:3000", true},
		{"chrome-extension://abcdef", "chrome-extension://abcdef", true},
		{"chrome-extension://otra", "chrome-extension://abcdef", false},
		{"https://x.ejemplo.es", "https://*.ejemplo.es", true},
	}
	for _, c := range casos {
		if got := originMatches(c.origin, c.patron); got != c.want {
			t.Errorf("originMatches(%q, %q) = %v, want %v", c.origin, c.patron, got, c.want)
		}
	}
}

func TestMigracionSemillaEndurecida_RetiraLoopbackYDominioPrivado(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	antiguas := map[string]string{
		"*.guadaltel.es":   "allowed",
		"localhost":        "allowed",
		"127.0.0.1":        "allowed",
		"::1":              "allowed",
		"*.gob.es":         "allowed",
		"sede.propia.es":   "allowed",
		"otro.denegado.es": "denied",
	}
	data, _ := json.Marshal(antiguas)
	if err := os.WriteFile(filepath.Join(dir, userDecisionsFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SeedMarkerPath(dir), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := NewWithOptions(dir, Options{SystemAllowlistFile: filepath.Join(dir, "no-existe.json"), TOFUEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, origen := range []string{"https://demos.guadaltel.es", "http://localhost:8080", "https://127.0.0.1:9999"} {
		if dec, _ := g.Evaluate(ctx, origen); dec.Status == domain.TrustAllowed {
			t.Errorf("%s no debería seguir autorizado tras la migración", origen)
		}
	}
	for _, origen := range []string{"https://sede.ejemplo.gob.es", "https://sede.propia.es", "https://127.0.0.1:63118"} {
		if dec, _ := g.Evaluate(ctx, origen); dec.Status != domain.TrustAllowed {
			t.Errorf("%s debería seguir autorizado, estado %s", origen, dec.Status)
		}
	}
	if dec, _ := g.Evaluate(ctx, "https://otro.denegado.es"); dec.Status != domain.TrustDenied {
		t.Errorf("la denegación explícita debe conservarse, estado %s", dec.Status)
	}
}
