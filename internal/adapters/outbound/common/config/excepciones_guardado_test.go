// SPDX-License-Identifier: EUPL-1.2
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCrearFicheroNoReescribeExcepcionesExistentes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	existing := []byte(`{"excepciones_inseguras":{"tsa_http":false},"excepciones_inseguras_aceptaciones":{"tsa_http":{"text_version":"2026-11-15.1","origin":"aceptado desde el aviso"}},"idioma":"es"}`)
	if err := os.WriteFile(path, existing, 0600); err != nil {
		t.Fatal(err)
	}
	if err := crearFichero(path, Config{ExcepcionesInseguras: map[string]bool{"tsa_http": true}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("JSON inválido")
	}
	var chosen map[string]bool
	json.Unmarshal(doc["excepciones_inseguras"], &chosen)
	if chosen["tsa_http"] {
		t.Fatal("creación de defaults restauró excepción desactivada")
	}
	if string(doc["excepciones_inseguras_aceptaciones"]) != `{"tsa_http":{"text_version":"2026-11-15.1","origin":"aceptado desde el aviso"}}` {
		t.Fatal("recibo manual perdido")
	}
}
func TestCrearFicheroNoIntroduceExcepcionSinRecibo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := crearFichero(path, Config{ExcepcionesInseguras: map[string]bool{"tsa_http": true}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	json.Unmarshal(raw, &doc)
	if len(doc["excepciones_inseguras"]) != 0 {
		t.Fatalf("default introdujo excepción sin recibo: %s", raw)
	}
}
func TestCrearFicheroRechazaAliasDeRecibo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"EXCEPCIONES_INSEGURAS_ACEPTACIONES":{"tsa_http":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := crearFichero(path, Default()); err == nil {
		t.Fatal("alias reservado aceptado")
	}
}
