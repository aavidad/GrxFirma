// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package certselectionstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Una carpeta de sesión abierta a otros (por ejemplo, creada por otro proceso
// con permisos amplios) no se lee: alguien podría haber plantado ahí qué
// certificado se elige solo para un origen.
func TestLoadSessionIgnoraCarpetaNoPrivada(t *testing.T) {
	perfilSinXDG(t)
	ruta, err := SessionPath()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(ruta)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(`{"https://malo.example":"cert-victima"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	store := New(t.TempDir())
	if id, ok, err := store.LoadSession(context.Background(), "https://malo.example"); err != nil || ok || id != "" {
		t.Fatalf("preferencia de carpeta no privada usada: %q, %v, %v", id, ok, err)
	}
	// Guardar la vuelve a dejar privada.
	if err := store.SaveSession(context.Background(), "https://portal.example", "cert"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("permisos %o tras guardar", info.Mode().Perm())
	}
}

// La carpeta de sesión no puede ser un enlace a otro sitio.
func TestSaveSessionRechazaCarpetaEnlace(t *testing.T) {
	perfilSinXDG(t)
	ruta, err := SessionPath()
	if err != nil {
		t.Fatal(err)
	}
	fuera := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(ruta)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fuera, filepath.Dir(ruta)); err != nil {
		t.Fatal(err)
	}
	store := New(t.TempDir())
	if err := store.SaveSession(context.Background(), "https://portal.example", "cert"); err == nil {
		t.Fatal("SaveSession siguió el enlace")
	}
	if entries, _ := os.ReadDir(fuera); len(entries) != 0 {
		t.Fatalf("se escribió a través del enlace: %v", entries)
	}
}
