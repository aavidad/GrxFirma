// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
)

// TestDefault_ValoresSegurosPorDefecto verifica que los defaults cierran todos los frentes de red.
func TestDefault_ValoresSegurosPorDefecto(t *testing.T) {
	t.Parallel()

	cfg := config.Default()

	if cfg.WebsocketHabilitado {
		t.Error("websocket_habilitado debe ser false por defecto (modelo Zero Server)")
	}
	if cfg.RestHabilitado {
		t.Error("rest_habilitado debe ser false por defecto (modelo Zero Server)")
	}
	if !cfg.TofuHabilitado {
		t.Error("tofu_habilitado debe ser true por defecto")
	}
	if cfg.NivelLog != "info" {
		t.Errorf("nivel_log = %q, want %q", cfg.NivelLog, "info")
	}
	if cfg.TimeoutOperacionSegundos != 30 {
		t.Errorf("timeout_operacion_segundos = %d, want 30", cfg.TimeoutOperacionSegundos)
	}
	if cfg.MaxTamanoDocumentoBytes != 52428800 {
		t.Errorf("max_tamano_documento_bytes = %d, want 52428800", cfg.MaxTamanoDocumentoBytes)
	}
}

// TestLoad_FicheroNoExiste_CreaConDefaults verifica que si no hay fichero se crea con los defaults.
func TestLoad_FicheroNoExiste_CreaConDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WebsocketHabilitado {
		t.Error("websocket_habilitado debe ser false tras crear fichero por primera vez")
	}
	if cfg.RestHabilitado {
		t.Error("rest_habilitado debe ser false tras crear fichero por primera vez")
	}

	// El fichero debe haberse creado
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("fichero config.json no creado: %v", err)
	}
	var leido config.Config
	if err := json.Unmarshal(data, &leido); err != nil {
		t.Fatalf("config.json creado no es JSON válido: %v", err)
	}
	if leido.WebsocketHabilitado {
		t.Error("config.json creado tiene websocket_habilitado=true")
	}
}

// TestLoad_FicheroExiste_CargaValores verifica que si el fichero existe sus valores se aplican.
func TestLoad_FicheroExiste_CargaValores(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	contenido := `{"websocket_habilitado": true, "rest_habilitado": false, "nivel_log": "debug"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contenido), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.WebsocketHabilitado {
		t.Error("websocket_habilitado debe ser true según el fichero")
	}
	if cfg.NivelLog != "debug" {
		t.Errorf("nivel_log = %q, want %q", cfg.NivelLog, "debug")
	}
}

// TestLoad_VariableEntornoSobreescribeFichero verifica que GRXFIRMA_* tiene precedencia sobre fichero.
func TestLoad_VariableEntornoSobreescribeFichero(t *testing.T) {
	// No paralelo: modifica variables de entorno del proceso.
	dir := t.TempDir()

	// Fichero dice websocket=true
	contenido := `{"websocket_habilitado": true}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contenido), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Env var dice websocket=false
	t.Setenv("GRXFIRMA_WEBSOCKET_HABILITADO", "false")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WebsocketHabilitado {
		t.Error("la variable de entorno GRXFIRMA_WEBSOCKET_HABILITADO=false debe prevalecer sobre el fichero")
	}
}

// TestLoad_EntornoActivaWebsocket verifica que la var de entorno puede activar el websocket.
func TestLoad_EntornoActivaWebsocket(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("GRXFIRMA_WEBSOCKET_HABILITADO", "true")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.WebsocketHabilitado {
		t.Error("GRXFIRMA_WEBSOCKET_HABILITADO=true debe activar websocket aunque el fichero diga false")
	}
}

// TestLoad_EntornoInvalido_NoPanic verifica que un valor no parseables no produce panic.
func TestLoad_EntornoInvalido_NoPanic(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("GRXFIRMA_WEBSOCKET_HABILITADO", "no-es-un-bool")
	t.Setenv("GRXFIRMA_TIMEOUT_OPERACION_SEGUNDOS", "no-es-un-numero")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Valores inválidos en entorno se ignoran; quedan los del fichero/defecto
	if cfg.WebsocketHabilitado {
		t.Error("valor inválido de entorno no debe activar websocket")
	}
	if cfg.TimeoutOperacionSegundos != 30 {
		t.Errorf("valor inválido de entorno debe mantener timeout por defecto; got %d", cfg.TimeoutOperacionSegundos)
	}
}

// TestLoad_PolicySobreescribeEntornoYFichero verifica que policy.json tiene máxima precedencia.
func TestLoad_PolicySobreescribeEntornoYFichero(t *testing.T) {
	dir := t.TempDir()

	// Fichero de usuario activa websocket
	contenido := `{"websocket_habilitado": true}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contenido), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Env var también lo activa
	t.Setenv("GRXFIRMA_WEBSOCKET_HABILITADO", "true")

	// Policy de organización lo desactiva — debe ganar
	policyDir := t.TempDir()
	policy := `{"websocket_habilitado": false}`
	if err := os.WriteFile(filepath.Join(policyDir, "policy.json"), []byte(policy), 0o644); err != nil {
		t.Fatalf("WriteFile(policy) error = %v", err)
	}

	// Para testear la policy necesitamos inyectar el directorio del sistema.
	// Usamos la función LoadWithPolicyDir en vez de Load para no depender de /etc.
	cfg, err := config.LoadWithPolicyDir(dir, policyDir)
	if err != nil {
		t.Fatalf("LoadWithPolicyDir() error = %v", err)
	}

	if cfg.WebsocketHabilitado {
		t.Error("policy.json con websocket_habilitado=false debe prevalecer sobre fichero y entorno")
	}
}

func TestLoad_WebsocketPermitidoPolicyPrevalece(t *testing.T) {
	userDir := t.TempDir()
	policyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(`{"websocket_habilitado":true,"websocket_permitido":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRXFIRMA_WEBSOCKET_PERMITIDO", "true")
	if err := os.WriteFile(filepath.Join(policyDir, "policy.json"), []byte(`{"websocket_permitido":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPolicyDir(userDir, policyDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PermiteWebSocket() {
		t.Fatal("websocket_permitido=false de política debe bloquear ambos modos")
	}
	if !cfg.WebsocketHabilitado {
		t.Fatal("la política de permiso no debe alterar el ajuste del residente")
	}
	if !(config.Config{}).PermiteWebSocket() {
		t.Fatal("una configuración antigua sin el nuevo campo debe conservar compatibilidad")
	}
}

func TestLoadPolicy_LeeDominiosDefinidosPorOrganizacion(t *testing.T) {
	t.Parallel()

	policyDir := t.TempDir()
	policy := `{
	  "tofu_habilitado": false,
	  "dominios_de_confianza": ["*.dipgra.es", "https://sede.example.org"]
	}`
	if err := os.WriteFile(filepath.Join(policyDir, "policy.json"), []byte(policy), 0o644); err != nil {
		t.Fatalf("WriteFile(policy) error = %v", err)
	}

	raw, err := config.LoadPolicy(policyDir)
	if err != nil {
		t.Fatalf("LoadPolicy() error = %v", err)
	}
	if raw.TofuHabilitado == nil || *raw.TofuHabilitado {
		t.Fatal("LoadPolicy debe conservar tofu_habilitado=false definido en policy.json")
	}
	if len(raw.DominiosDeConfianza) != 2 {
		t.Fatalf("dominios_de_confianza = %v, want 2 entradas", raw.DominiosDeConfianza)
	}
	if raw.DominiosDeConfianza[0] != "*.dipgra.es" {
		t.Fatalf("primer dominio = %q, want %q", raw.DominiosDeConfianza[0], "*.dipgra.es")
	}
}

func TestLoad_PolicyPuedeDeshabilitarTOFU(t *testing.T) {
	dir := t.TempDir()
	policyDir := t.TempDir()
	policy := `{"tofu_habilitado": false}`
	if err := os.WriteFile(filepath.Join(policyDir, "policy.json"), []byte(policy), 0o644); err != nil {
		t.Fatalf("WriteFile(policy) error = %v", err)
	}

	cfg, err := config.LoadWithPolicyDir(dir, policyDir)
	if err != nil {
		t.Fatalf("LoadWithPolicyDir() error = %v", err)
	}
	if cfg.TofuHabilitado {
		t.Error("policy.json con tofu_habilitado=false debe deshabilitar TOFU")
	}
}
