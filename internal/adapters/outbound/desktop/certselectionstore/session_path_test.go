// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certselectionstore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func perfilSinXDG(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_RUNTIME_DIR", "")
	return home
}

// Sin XDG_RUNTIME_DIR las preferencias de sesión van a una carpeta privada
// dentro del perfil, creada con 0700, y nunca a la carpeta temporal.
func TestSessionPathSinXDGQuedaEnElPerfil(t *testing.T) {
	home := perfilSinXDG(t)
	ruta, err := SessionPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ruta, home+string(filepath.Separator)) {
		t.Fatalf("ruta de sesión fuera del perfil: %q", ruta)
	}
	store := New(t.TempDir())
	if err := store.SaveSession(context.Background(), "https://portal.example", "cert-sesion"); err != nil {
		t.Fatal(err)
	}
	if id, ok, err := store.LoadSession(context.Background(), "https://portal.example"); err != nil || !ok || id != "cert-sesion" {
		t.Fatalf("LoadSession = %q, %v, %v", id, ok, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Dir(ruta))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("permisos de la carpeta de sesión %o", info.Mode().Perm())
		}
	}
}

// Sin carpeta personal no hay preferencias de sesión: cargar no encuentra nada
// y guardar falla, pero nada se escribe en la carpeta temporal.
func TestSessionSinCarpetaPersonalFallaCerrado(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	if ruta, err := SessionPath(); err == nil {
		t.Fatalf("SessionPath sin carpeta personal = %q", ruta)
	}
	store := New(t.TempDir())
	if id, ok, err := store.LoadSession(context.Background(), "https://portal.example"); err != nil || ok || id != "" {
		t.Fatalf("LoadSession = %q, %v, %v", id, ok, err)
	}
	if err := store.SaveSession(context.Background(), "https://portal.example", "cert"); err == nil {
		t.Fatal("SaveSession guardó sin carpeta privada")
	}
}
