// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
)

func TestFirmaRemotaCSCDesactivadaPorDefecto(t *testing.T) {
	cfg := config.Default()
	if cfg.FirmaRemotaCSC || cfg.FirmaRemotaCSCActiva(config.Policy{}) {
		t.Fatal("la firma remota CSC debe venir desactivada")
	}
}

func TestFirmaRemotaCSCLaPoliticaMandaYSinElLaDecideConfigJSON(t *testing.T) {
	falso, verdadero := false, true
	cfg := config.Default()
	if !cfg.FirmaRemotaCSCActiva(config.Policy{FirmaRemotaCSC: &verdadero}) {
		t.Fatal("una política que la permite debe activarla")
	}
	cfg.FirmaRemotaCSC = true
	if !cfg.FirmaRemotaCSCActiva(config.Policy{}) {
		t.Fatal("sin política, config.json debe activarla")
	}
	if cfg.FirmaRemotaCSCActiva(config.Policy{FirmaRemotaCSC: &falso}) {
		t.Fatal("la política de la organización debe poder prohibirla")
	}
	if !config.FirmaRemotaCSCProhibida(config.Policy{FirmaRemotaCSC: &falso}) ||
		config.FirmaRemotaCSCProhibida(config.Policy{FirmaRemotaCSC: &verdadero}) ||
		config.FirmaRemotaCSCProhibida(config.Policy{}) {
		t.Fatal("solo una política explícita en contra cuenta como prohibición")
	}
}

func TestFirmaRemotaCSCNoSeActivaPorEntornoYLaPoliticaFicheroSeAplica(t *testing.T) {
	t.Setenv("GRXFIRMA_FIRMA_REMOTA_CSC", "true")
	usuario, politica := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(usuario, "config.json"), []byte(`{"firma_remota_csc": true, "firma_remota_csc_oauth_permitidos": ["a.example=b.example"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPolicyDir(usuario, politica)
	if err != nil || !cfg.FirmaRemotaCSC || !reflect.DeepEqual(cfg.FirmaRemotaCSCOAuth, []string{"a.example=b.example"}) {
		t.Fatalf("config.json no se aplicó: %v %+v", err, cfg)
	}
	if err := os.WriteFile(filepath.Join(politica, "policy.json"), []byte(`{"firma_remota_csc": false, "firma_remota_csc_oauth_permitidos": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.LoadWithPolicyDir(usuario, politica)
	if err != nil || cfg.FirmaRemotaCSC || len(cfg.FirmaRemotaCSCOAuth) != 0 {
		t.Fatalf("policy.json debe prevalecer: %v %+v", err, cfg)
	}
	pol, err := config.LoadPolicy(politica)
	if err != nil || pol.FirmaRemotaCSC == nil || *pol.FirmaRemotaCSC || cfg.FirmaRemotaCSCActiva(pol) {
		t.Fatalf("LoadPolicy debe exponer la prohibición: %v %+v", err, pol)
	}
	vacio := t.TempDir()
	cfg, err = config.LoadWithPolicyDir(vacio, t.TempDir())
	if err != nil || cfg.FirmaRemotaCSC {
		t.Fatalf("la variable de entorno no debe activarla: %v %+v", err, cfg)
	}
}
