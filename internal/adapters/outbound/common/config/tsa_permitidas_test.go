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

// Lote 13a: la lista de TSA permitidas solo la fija la política de máquina.
func TestPolicyTSAPermitidas(t *testing.T) {
	if cfg := cargarConPolicy(t, ""); cfg.TSAPermitidas != nil {
		t.Fatalf("sin política la lista no está definida: %v", cfg.TSAPermitidas)
	}
	cfg := cargarConPolicy(t, `{"tsa_permitidas": ["https://tsa.organismo.example/tsa"]}`)
	if len(cfg.TSAPermitidas) != 1 || cfg.TSAPermitidas[0] != "https://tsa.organismo.example/tsa" {
		t.Fatalf("lista de la política = %v", cfg.TSAPermitidas)
	}
	if cfg := cargarConPolicy(t, `{"tsa_permitidas": []}`); cfg.TSAPermitidas == nil || len(cfg.TSAPermitidas) != 0 {
		t.Fatalf("una lista vacía cuenta como definida: %#v", cfg.TSAPermitidas)
	}
	// Ilegible: definida y vacía (falla cerrado en REST).
	if cfg := cargarConPolicy(t, `{"tsa_permitidas": "https://tsa.example"}`); cfg.TSAPermitidas == nil || len(cfg.TSAPermitidas) != 0 {
		t.Fatalf("una lista ilegible debe quedar definida y vacía: %#v", cfg.TSAPermitidas)
	}
}

// El fichero del usuario no puede definir la lista.
func TestConfigUsuarioNoDefineTSAPermitidas(t *testing.T) {
	userDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(`{"tsa_permitidas": ["http://127.0.0.1"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPolicyDir(userDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TSAPermitidas != nil {
		t.Fatalf("el usuario no debe poder definir la lista: %v", cfg.TSAPermitidas)
	}
}
