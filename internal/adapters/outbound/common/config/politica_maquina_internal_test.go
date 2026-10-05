// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"reflect"
	"testing"

	"grxfirma/internal/security/machinepolicy"
)

// La política de máquina de Windows (registro) debe poder prohibir o
// permitir la firma remota CSC y fijar los pares OAuth.
func TestPoliticaMaquinaLeeFirmaRemotaCSC(t *testing.T) {
	registro := map[string]any{
		machinepolicy.FirmaRemotaCSC:      int64(0),
		machinepolicy.FirmaRemotaCSCOAuth: []string{"firma.example=auth.example"},
	}
	lector := lectorPoliticaMaquina{
		boolean: func(n string) (bool, bool, error) {
			v, ok := registro[n].(int64)
			return v != 0, ok, nil
		},
		texto:  func(string) (string, bool, error) { return "", false, nil },
		textos: func(n string) ([]string, bool, error) { v, ok := registro[n].([]string); return v, ok, nil },
		entero: func(n string) (int64, bool, error) { v, ok := registro[n].(int64); return v, ok, nil },
	}
	p, err := loadMachinePolicyDesde(lector)
	if err != nil || p.FirmaRemotaCSC == nil || *p.FirmaRemotaCSC {
		t.Fatalf("firma_remota_csc=0 debe leerse como prohibición: %v %+v", err, p)
	}
	if !reflect.DeepEqual(p.FirmaRemotaCSCOAuth, []string{"firma.example=auth.example"}) {
		t.Fatalf("pares OAuth: %v", p.FirmaRemotaCSCOAuth)
	}
	cfg := Default()
	cfg.FirmaRemotaCSC = true
	aplicarPolicyCargada(&cfg, p, nil)
	if cfg.FirmaRemotaCSC || cfg.FirmaRemotaCSCActiva(p) {
		t.Fatal("la política de máquina debe prevalecer sobre config.json")
	}

	registro[machinepolicy.FirmaRemotaCSC] = int64(1)
	p, _ = loadMachinePolicyDesde(lector)
	if !Default().FirmaRemotaCSCActiva(p) {
		t.Fatal("firma_remota_csc=1 debe permitirla")
	}
	delete(registro, machinepolicy.FirmaRemotaCSC)
	p, _ = loadMachinePolicyDesde(lector)
	if p.FirmaRemotaCSC != nil {
		t.Fatal("sin valor en el registro no hay decisión de la organización")
	}
}
