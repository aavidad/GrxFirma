// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"errors"
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

	// ID-03: la lista de prestadores; fijada y vacía no admite ninguno.
	if p.FirmaRemotaCSCServicios != nil {
		t.Fatal("sin valor en el registro no hay límite de prestadores")
	}
	registro[machinepolicy.FirmaRemotaCSCServicios] = []string(nil)
	if p, _ = loadMachinePolicyDesde(lector); p.FirmaRemotaCSCServicios == nil || len(p.FirmaRemotaCSCServicios) != 0 {
		t.Fatalf("una lista fijada y vacía debe limitar: %#v", p.FirmaRemotaCSCServicios)
	}
	registro[machinepolicy.FirmaRemotaCSCServicios] = []string{"firma.example.es"}
	if p, _ = loadMachinePolicyDesde(lector); !reflect.DeepEqual(p.FirmaRemotaCSCServicios, []string{"firma.example.es"}) {
		t.Fatalf("servicios: %v", p.FirmaRemotaCSCServicios)
	}
}

// Lote 14: prohibir_confianza_forzada en el registro de Windows. Un valor
// que no se puede leer cuenta como prohibición, igual que con TOFU.
func TestPoliticaMaquinaLeeProhibirConfianzaForzada(t *testing.T) {
	registro := map[string]any{machinepolicy.ProhibirConfianzaForzada: int64(1)}
	lector := lectorPoliticaMaquina{
		boolean: func(n string) (bool, bool, error) {
			switch v := registro[n].(type) {
			case int64:
				return v != 0, true, nil
			case error:
				return false, false, v
			}
			return false, false, nil
		},
		texto:  func(string) (string, bool, error) { return "", false, nil },
		textos: func(string) ([]string, bool, error) { return nil, false, nil },
		entero: func(string) (int64, bool, error) { return 0, false, nil },
	}
	prohibida := func() bool {
		p, err := loadMachinePolicyDesde(lector)
		cfg := Default()
		aplicarPolicyCargada(&cfg, p, err)
		return cfg.ProhibirConfianzaForzada
	}
	if !prohibida() {
		t.Fatal("prohibir_confianza_forzada=1 debe prohibir")
	}
	registro[machinepolicy.ProhibirConfianzaForzada] = int64(0)
	if prohibida() {
		t.Fatal("prohibir_confianza_forzada=0 debe permitir")
	}
	delete(registro, machinepolicy.ProhibirConfianzaForzada)
	if prohibida() {
		t.Fatal("sin valor en el registro no hay prohibición")
	}
	registro[machinepolicy.ProhibirConfianzaForzada] = errors.New("REG_SZ donde se espera REG_DWORD")
	if !prohibida() {
		t.Fatal("un valor ilegible debe contar como prohibición")
	}
}
