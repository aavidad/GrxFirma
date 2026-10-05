// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigYPoliticaConBOMUTF8(t *testing.T) {
	dir := t.TempDir()
	bom := "\xEF\xBB\xBF"
	user := filepath.Join(dir, "config.json")
	if err := os.WriteFile(user, []byte(bom+`{"firma_remota_csc": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := cargarFicheroUsuario(user, &cfg); err != nil {
		t.Fatalf("config.json con BOM debe leerse: %v", err)
	}
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policy, []byte(bom+`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicyFile(policy); err != nil {
		t.Fatalf("policy.json con BOM debe leerse: %v", err)
	}
}
