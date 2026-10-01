// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package sandboxstorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNuevoStore_Exito(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Nuevo(filepath.Join(dir, "sandbox"))
	if err != nil {
		t.Fatalf("Nuevo() error = %v", err)
	}
	if store.baseDir == "" {
		t.Fatal("baseDir vacio")
	}
}

func TestStoreWriteYDelete_Exito(t *testing.T) {
	t.Parallel()

	store, err := Nuevo(t.TempDir())
	if err != nil {
		t.Fatalf("Nuevo() error = %v", err)
	}

	ruta, err := store.Write(context.Background(), []byte("resultado"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(ruta); err != nil {
		t.Fatalf("Stat(%q) error = %v", ruta, err)
	}

	if err := store.Delete(context.Background(), ruta); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(ruta); !os.IsNotExist(err) {
		t.Fatalf("el temporal sigue existiendo: err=%v", err)
	}
}

func TestStoreWrite_DatosVacios(t *testing.T) {
	t.Parallel()

	store, err := Nuevo(t.TempDir())
	if err != nil {
		t.Fatalf("Nuevo() error = %v", err)
	}

	if _, err := store.Write(context.Background(), nil); err == nil {
		t.Fatal("Write() error = nil, want error")
	}
}

func TestStoreDelete_FueraDelSandbox(t *testing.T) {
	t.Parallel()

	store, err := Nuevo(t.TempDir())
	if err != nil {
		t.Fatalf("Nuevo() error = %v", err)
	}

	if err := store.Delete(context.Background(), filepath.Join(t.TempDir(), "ajeno.tmp")); err == nil {
		t.Fatal("Delete() error = nil, want error")
	}
}

func TestNuevoStore_RechazaAlmacenamientoCompartidoAndroid(t *testing.T) {
	t.Parallel()

	for _, ruta := range []string{
		"/sdcard/Download",
		"/storage/emulated/0/Download",
		"/storage/self/primary/Documents",
		"/mnt/sdcard/tmp",
	} {
		store, err := Nuevo(ruta)
		if err == nil || store != nil {
			t.Fatalf("Nuevo(%q) debía fallar por almacenamiento compartido", ruta)
		}
	}
}
