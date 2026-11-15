// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package usersettings

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Dos instancias, como la bandeja y el motor, pueden guardar instantáneas
// obtenidas en instantes distintos. La desactivación reciente debe prevalecer.
func TestGuardarNoResucitaExcepcionDeInstantaneaAntigua(t *testing.T) {
	dir := t.TempDir()
	initial := []byte(`{"idioma":"es","excepciones_inseguras":{"tsa_http":true},"excepciones_inseguras_aceptaciones":{"tsa_http":{"id":"tsa_http","accepted_at":"2026-11-15T00:00:00Z","text_version":"2026-11-15.1","origin":"aceptado desde el aviso","channel":"cli"}}}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), initial, 0600); err != nil {
		t.Fatal(err)
	}
	tray := New(dir)
	old, err := tray.Cargar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"idioma":"es","excepciones_inseguras":{"tsa_http":false},"excepciones_inseguras_aceptaciones":{"tsa_http":{"id":"tsa_http","accepted_at":"2026-11-15T00:00:00Z","text_version":"2026-11-15.1","origin":"aceptado desde el aviso","channel":"cli"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	old["idioma"] = "en"
	if err := tray.Guardar(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	fresh, err := tray.Cargar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	choices, ok := fresh["excepciones_inseguras"].(map[string]any)
	if !ok || choices["tsa_http"] != false {
		t.Fatalf("la instantánea antigua reactivó la excepción: %#v", fresh)
	}
	if fresh["idioma"] != "en" {
		t.Fatalf("la preferencia normal no se guardó: %#v", fresh)
	}
}
func TestGuardarGeneralNoIntroduceExcepciones(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.Guardar(context.Background(), map[string]any{"idioma": "es", "excepciones_inseguras": map[string]any{"tsa_http": true}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Cargar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["excepciones_inseguras"]; ok {
		t.Fatalf("guardado genérico introdujo el mapa: %#v", got)
	}
}

func TestGuardarRechazaAliasDeAjustesReservados(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	if err := store.Guardar(context.Background(), map[string]any{"EXCEPCIONES_INSEGURAS": map[string]bool{"tsa_http": true}}); err == nil {
		t.Fatal("alias de ajuste permitido")
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("alias escribió preferencias: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"EXCEPCIONES_INSEGURAS_ACEPTACIONES":{"tsa_http":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Guardar(context.Background(), map[string]any{"idioma": "es"}); err == nil {
		t.Fatal("alias en disco aceptado")
	}
}
