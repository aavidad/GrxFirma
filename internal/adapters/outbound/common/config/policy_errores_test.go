// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
)

func cargarConPolicy(t *testing.T, contenido string) config.Config {
	t.Helper()
	userDir := t.TempDir()
	policyDir := t.TempDir()
	if contenido != "" {
		if err := os.WriteFile(filepath.Join(policyDir, "policy.json"), []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// El usuario intenta activar REST y WebSocket por entorno.
	t.Setenv("GRXFIRMA_REST_HABILITADO", "true")
	t.Setenv("GRXFIRMA_WEBSOCKET_HABILITADO", "true")
	cfg, err := config.LoadWithPolicyDir(userDir, policyDir)
	if err != nil {
		t.Fatalf("LoadWithPolicyDir(): %v", err)
	}
	return cfg
}

// F-07: un valor con el tipo equivocado en un ajuste no descarta el resto.
func TestPolicyUnCampoInvalidoNoAnulaLosDemas(t *testing.T) {
	cfg := cargarConPolicy(t, `{"rest_habilitado": false, "nivel_log": 5}`)
	if cfg.RestHabilitado {
		t.Fatal("rest_habilitado=false de la política debe aplicarse aunque nivel_log sea inválido")
	}
	if cfg.NivelLog != "info" {
		t.Fatalf("nivel_log inválido no debe cambiar el valor: %q", cfg.NivelLog)
	}
}

// F-07: un ajuste restrictivo ilegible falla cerrado, no abierto.
func TestPolicyAjusteRestrictivoInvalidoFallaCerrado(t *testing.T) {
	cfg := cargarConPolicy(t, `{"rest_habilitado": "no", "websocket_habilitado": 7}`)
	if cfg.RestHabilitado || cfg.WebsocketHabilitado {
		t.Fatalf("REST/WebSocket deben quedar desactivados: rest=%v ws=%v", cfg.RestHabilitado, cfg.WebsocketHabilitado)
	}
}

// F-07: una política que existe pero no es JSON desactiva REST y WebSocket.
func TestPolicyIlegibleDesactivaRestYWebsocket(t *testing.T) {
	cfg := cargarConPolicy(t, `{no es json`)
	if cfg.RestHabilitado || cfg.WebsocketHabilitado || cfg.PermiteWebSocket() {
		t.Fatalf("política ilegible: rest=%v ws=%v permitido=%v", cfg.RestHabilitado, cfg.WebsocketHabilitado, cfg.PermiteWebSocket())
	}
}

// Sin política no cambia nada: el entorno del usuario se respeta.
func TestPolicyAusenteNoCierraNada(t *testing.T) {
	cfg := cargarConPolicy(t, "")
	if !cfg.RestHabilitado || !cfg.WebsocketHabilitado {
		t.Fatalf("sin política deben valer los ajustes del usuario: rest=%v ws=%v", cfg.RestHabilitado, cfg.WebsocketHabilitado)
	}
}

// F-07: sin política fiable tampoco se amplía la confianza con TOFU.
func TestPolicyIlegibleODominiosInvalidosDesactivanTOFU(t *testing.T) {
	for nombre, contenido := range map[string]string{
		"ilegible":           `{no es json`,
		"dominios_invalidos": `{"dominios_de_confianza": "valide.redsara.es"}`,
	} {
		cfg := cargarConPolicy(t, contenido)
		if cfg.TofuHabilitado {
			t.Fatalf("%s: TOFU debe quedar desactivado", nombre)
		}
	}
}

// F-07 sobre la firma remota CSC de la 0.0.125: una política que existe pero
// no se puede leer, o que trae esos ajustes con el tipo equivocado, no deja que
// config.json del usuario active la firma remota ni amplíe los servidores OAuth.
func TestPolicyInvalidaCierraLaFirmaRemotaCSCDelUsuario(t *testing.T) {
	for nombre, contenido := range map[string]string{
		"ilegible":       `{no es json`,
		"csc_invalido":   `{"firma_remota_csc": "si", "firma_remota_csc_oauth_permitidos": 3}`,
		"oauth_invalido": `{"firma_remota_csc_oauth_permitidos": "a.example=b.example"}`,
	} {
		usuario, politica := t.TempDir(), t.TempDir()
		if err := os.WriteFile(filepath.Join(usuario, "config.json"), []byte(`{"firma_remota_csc": true, "firma_remota_csc_oauth_permitidos": ["a.example=b.example"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(politica, "policy.json"), []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.LoadWithPolicyDir(usuario, politica)
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		if nombre != "oauth_invalido" && cfg.FirmaRemotaCSC {
			t.Errorf("%s: la firma remota CSC debe quedar desactivada", nombre)
		}
		if nombre != "ilegible" && len(cfg.FirmaRemotaCSCOAuth) != 0 {
			t.Errorf("%s: la lista OAuth del usuario no debe sobrevivir a una política inválida: %v", nombre, cfg.FirmaRemotaCSCOAuth)
		}
	}
}
