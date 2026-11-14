// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
)

func escribirPolicy(t *testing.T, contenido string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPoliticaProgramaAfirmaDelFichero(t *testing.T) {
	casos := map[string]ports.PoliticaProgramaAfirma{
		`{}`:                                  {},
		`{"manejador_afirma": "AutoFirma"}`:   {Valor: ports.ProgramaAfirmaAutoFirma},
		`{"manejador_afirma": " preguntar "}`: {Valor: ports.ProgramaAfirmaPreguntar},
		`{"manejador_afirma": "grxfirma"}`:    {Valor: ports.ProgramaAfirmaGrxFirma},
		`{"manejador_afirma": "otro"}`:        {Ilegible: true},
		`{"manejador_afirma": 1}`:             {Ilegible: true},
		`{no es json`:                         {Ilegible: true},
		`{"rest_habilitado": "x"}`:            {},
		`{"manejador_afirma": "autofirma", "rest_habilitado": "x"}`: {Valor: ports.ProgramaAfirmaAutoFirma},
	}
	for contenido, esperado := range casos {
		if got := PoliticaProgramaAfirma(escribirPolicy(t, contenido)); got != esperado {
			t.Errorf("%s: %+v, se esperaba %+v", contenido, got, esperado)
		}
	}
	if got := PoliticaProgramaAfirma(t.TempDir()); got != (ports.PoliticaProgramaAfirma{}) {
		t.Fatalf("sin policy.json no hay política: %+v", got)
	}
}

func TestPoliticaProgramaAfirmaDelRegistro(t *testing.T) {
	valor := "preguntar"
	lector := lectorPoliticaMaquina{
		boolean: func(string) (bool, bool, error) { return false, false, nil },
		texto: func(n string) (string, bool, error) {
			if n == machinepolicy.ManejadorAfirma {
				return valor, true, nil
			}
			return "", false, nil
		},
		textos: func(string) ([]string, bool, error) { return nil, false, nil },
		entero: func(string) (int64, bool, error) { return 0, false, nil },
	}
	p, err := loadMachinePolicyDesde(lector)
	if err != nil || p.ManejadorAfirma == nil || *p.ManejadorAfirma != ports.ProgramaAfirmaPreguntar {
		t.Fatalf("manejador_afirma del registro: %v %+v", err, p)
	}
	valor = "AutoFirma.exe"
	p, err = loadMachinePolicyDesde(lector)
	if err == nil || p.ManejadorAfirma != nil {
		t.Fatalf("un valor no admitido debe invalidarse: %v %+v", err, p)
	}
}

// El usuario no puede fijar manejador_afirma desde su config.json: ese
// fichero no es la política.
func TestManejadorAfirmaNoSeLeeDelFicheroDeUsuario(t *testing.T) {
	usuario := t.TempDir()
	if err := os.WriteFile(filepath.Join(usuario, "config.json"), []byte(`{"manejador_afirma":"autofirma"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := PoliticaProgramaAfirma(t.TempDir()); got.Valor != "" {
		t.Fatalf("config.json del usuario no es política: %+v", got)
	}
}
