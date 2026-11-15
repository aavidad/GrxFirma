// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// La autenticación REST no acredita una aceptación en la interfaz local.
func TestRESTExcepcionesNoActivaPorRutaNiAjustesGenericos(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"excepciones_inseguras":{"sha1_crear":false},"idioma":"es"}`)
	for _, file := range []string{"config.json", "settings.json", "qt-settings.json"} {
		if err := os.WriteFile(filepath.Join(dir, file), original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	adapter := nuevoAdaptadorPruebas(nil, nil, nil).WithConfigDir(dir)
	handler := adapter.Routes()
	malicious := map[string]any{
		"id": "sha1_crear", "enabled": true, "accepted": true,
		"channel": "local_ipc", "source": "gui", "local": true,
		"excepciones_inseguras": map[string]bool{"sha1_crear": true},
	}
	for _, route := range []string{"/unsafe_exceptions_activate", "/unsafe_exceptions_set", "/unsafe_exceptions_retry", "/settings/unsafe-exceptions", "/v2/unsafe_exceptions_activate"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
			response := doJSON(t, handler, method, route, malicious)
			if response.Code < 400 {
				t.Fatalf("%s %s admite activación: %d %s", method, route, response.Code, response.Body.String())
			}
		}
	}
	response := doJSON(t, handler, http.MethodPost, "/settings", malicious)
	if response.Code < 400 {
		t.Fatalf("/settings debe rechazar el bloque reservado: %d %s", response.Code, response.Body.String())
	}
	for _, file := range []string{"config.json", "settings.json", "qt-settings.json"} {
		got, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("REST cambió %s: %s (%v)", file, got, err)
		}
	}
}
