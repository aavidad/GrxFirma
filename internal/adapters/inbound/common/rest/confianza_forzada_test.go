// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
)

// Lote 14: el canal REST no puede crear, cambiar ni leer lo que el usuario
// aceptó en su equipo al verificar. /settings escribe qt-settings.json y no
// toca config.json; no hay ninguna ruta de confianza forzada.
func TestREST_NoGestionaConfianzaForzada(t *testing.T) {
	dir := t.TempDir()
	huella := strings.Repeat("f", 64)
	previa := `{"confianza_forzada": [{"tipo": "ca_desconocida", "huella": "` + huella + `", "aceptada_en": "2026-11-13T00:00:00Z", "aviso": "verify.forced_trust.notice.ca_desconocida"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(previa), 0o600); err != nil {
		t.Fatal(err)
	}
	adaptador := nuevoAdaptadorPruebas(nil, nil, nil).WithConfigDir(dir)

	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/settings", nil)
	if rr.Code != http.StatusOK || bytes.Contains(rr.Body.Bytes(), []byte("confianza_forzada")) || bytes.Contains(rr.Body.Bytes(), []byte(huella)) {
		t.Fatalf("GET /settings no puede leer la confianza aceptada: %d %s", rr.Code, rr.Body.String())
	}

	nueva := map[string]any{"confianza_forzada": []map[string]any{
		{"tipo": "algoritmo_sha1", "aceptada_en": "2026-11-13T00:00:00Z", "aviso": "verify.forced_trust.notice.algoritmo_sha1"},
		{"tipo": "revocacion_no_comprobable", "aceptada_en": "2026-11-13T00:00:00Z", "aviso": "verify.forced_trust.notice.revocacion_no_comprobable"},
	}}
	doJSON(t, adaptador.Routes(), http.MethodPost, "/settings", nueva)
	confianza, err := config.NuevoAlmacenConfianzaForzadaConPolicy(dir, t.TempDir()).CargarConfianzaForzada(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(confianza.Aceptaciones) != 1 || confianza.Aceptaciones[0].Huella != huella {
		t.Fatalf("POST /settings cambió la confianza aceptada: %+v", confianza.Aceptaciones)
	}

	for _, ruta := range []string{"/forced-trust", "/forced_trust", "/forced_trust_accept", "/confianza-forzada", "/confianza_forzada", "/settings/forced-trust", "/v2/forced-trust"} {
		for _, metodo := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			rr := doJSON(t, adaptador.Routes(), metodo, ruta, map[string]any{"type": "algoritmo_sha1"})
			if rr.Code == http.StatusOK || bytes.Contains(rr.Body.Bytes(), []byte(huella)) {
				t.Fatalf("%s %s respondió %d: %s", metodo, ruta, rr.Code, rr.Body.String())
			}
		}
	}
	if datos, _ := os.ReadFile(filepath.Join(dir, "config.json")); !bytes.Equal(datos, []byte(previa)) {
		t.Fatalf("config.json cambió desde REST: %s", datos)
	}
}
