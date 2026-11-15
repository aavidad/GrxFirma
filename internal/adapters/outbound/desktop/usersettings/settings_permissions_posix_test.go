// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package usersettings_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/settingsfile"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
)

func TestGuardar_PermisosRestringidos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ruta := filepath.Join(dir, "settings.json")
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod(dir): %v", err)
	}
	if err := os.WriteFile(ruta, []byte(`{"anterior":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings): %v", err)
	}
	if err := os.Chmod(ruta, 0o644); err != nil {
		t.Fatalf("Chmod(settings): %v", err)
	}

	almacen := usersettings.New(dir)
	if err := almacen.Guardar(context.Background(), map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	infoDir, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if perm := infoDir.Mode().Perm(); perm != 0o700 {
		t.Errorf("permisos del directorio: got %o, queria 0700", perm)
	}
	infoFile, err := os.Stat(ruta)
	if err != nil {
		t.Fatalf("Stat(settings): %v", err)
	}
	if perm := infoFile.Mode().Perm(); perm != 0o600 {
		t.Errorf("permisos del fichero: got %o, queria 0600", perm)
	}
}

// Un settings.json que el usuario no puede abrir (por ejemplo, guardado con
// sudo) no puede acabar en un diagnóstico genérico: el error lleva la clave
// del catálogo que dice qué pasa y cómo arreglarlo.
func TestCargar_SinPermisoExplicaQuePasaYComoArreglarlo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lee cualquier fichero")
	}
	dir := t.TempDir()
	ruta := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(ruta, []byte(`{"idioma":"es"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ruta, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ruta, 0o600) })
	_, err := usersettings.New(dir).Cargar(context.Background())
	var explained *settingsfile.Error
	if !errors.As(err, &explained) || explained.Key != settingsfile.KeyNoPermission {
		t.Fatalf("Cargar = %v; se esperaba la clave %s", err, settingsfile.KeyNoPermission)
	}
	if got, want := err.Error(), localizador.Detectar().T(settingsfile.KeyNoPermission); got != want {
		t.Fatalf("texto %q; se esperaba el del catálogo %q", got, want)
	}
	err = usersettings.New(dir).Guardar(context.Background(), map[string]any{"idioma": "en"})
	if !errors.As(err, &explained) || explained.Key != settingsfile.KeyNoPermission {
		t.Fatalf("Guardar = %v; se esperaba la clave %s", err, settingsfile.KeyNoPermission)
	}
}
