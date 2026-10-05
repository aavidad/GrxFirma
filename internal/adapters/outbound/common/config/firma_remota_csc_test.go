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

func TestFirmaRemotaCSCDesactivadaPorDefecto(t *testing.T) {
	cfg := config.Default()
	if cfg.FirmaRemotaCSC || cfg.FirmaRemotaCSCActiva(false, config.Policy{}) {
		t.Fatal("la firma remota CSC debe venir desactivada")
	}
}

func TestFirmaRemotaCSCSeActivaPorFicheroOCLIYLaPoliticaManda(t *testing.T) {
	falso, verdadero := false, true
	cfg := config.Default()
	if !cfg.FirmaRemotaCSCActiva(true, config.Policy{}) {
		t.Fatal("la opción de la CLI debe activarla")
	}
	cfg.FirmaRemotaCSC = true
	if !cfg.FirmaRemotaCSCActiva(false, config.Policy{FirmaRemotaCSC: &verdadero}) {
		t.Fatal("config.json debe activarla")
	}
	if cfg.FirmaRemotaCSCActiva(true, config.Policy{FirmaRemotaCSC: &falso}) {
		t.Fatal("la política de la organización debe poder prohibirla")
	}
}

func TestFirmaRemotaCSCNoSeActivaPorEntornoYLaPoliticaFicheroSeAplica(t *testing.T) {
	t.Setenv("GRXFIRMA_FIRMA_REMOTA_CSC", "true")
	usuario, politica := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(usuario, "config.json"), []byte(`{"firma_remota_csc": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPolicyDir(usuario, politica)
	if err != nil || !cfg.FirmaRemotaCSC {
		t.Fatalf("config.json no se aplicó: %v %+v", err, cfg)
	}
	if err := os.WriteFile(filepath.Join(politica, "policy.json"), []byte(`{"firma_remota_csc": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.LoadWithPolicyDir(usuario, politica)
	if err != nil || cfg.FirmaRemotaCSC {
		t.Fatalf("policy.json debe prevalecer: %v %+v", err, cfg)
	}
	vacio := t.TempDir()
	cfg, err = config.LoadWithPolicyDir(vacio, t.TempDir())
	if err != nil || cfg.FirmaRemotaCSC {
		t.Fatalf("la variable de entorno no debe activarla: %v %+v", err, cfg)
	}
}
