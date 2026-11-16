// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultPathUsaCarpetaPrivadaDelUsuario(t *testing.T) {
	runtime := t.TempDir()
	if err := os.Chmod(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	// Una carpeta personal falsa: si XDG_RUNTIME_DIR no se admitiera, la
	// prueba no tocaría la del usuario real.
	t.Setenv("HOME", t.TempDir())

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(runtime, defaultDirName, defaultFileName)
	if path != want {
		t.Fatalf("DefaultPath() = %q, se esperaba %q", path, want)
	}
	if strings.HasPrefix(path, os.TempDir()+string(os.PathSeparator)+"grxfirma-metrics") {
		t.Fatalf("la ruta por defecto sigue en la carpeta temporal compartida: %q", path)
	}

	c := NewDefault()
	c.RecordProtocolRequest(context.Background(), "sign", "ok", time.Millisecond)
	info, err := os.Stat(filepath.Dir(want))
	if err != nil {
		t.Fatalf("no se creó la carpeta de métricas: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("permisos de la carpeta = %o, se esperaba 700", perm)
	}
	finfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("no se escribieron métricas: %v", err)
	}
	if perm := finfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permisos del fichero = %o, se esperaba 600", perm)
	}
}

func TestSinCarpetaPrivadaNoSeEscribeNada(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", "")

	if path, err := DefaultPath(); err == nil {
		t.Fatalf("DefaultPath() = %q sin carpeta privada; se esperaba error", path)
	}
	c := NewDefault()
	if c.path != "" {
		t.Fatalf("el recolector tiene ruta %q sin carpeta privada", c.path)
	}
	// No debe fallar ni escribir: sigue contando en memoria.
	c.RecordSign(context.Background(), "cades", "ok", time.Millisecond)
	if got := c.signTotal[signKey{Format: "cades", Status: "ok"}]; got != 1 {
		t.Fatalf("contador en memoria = %d, se esperaba 1", got)
	}
}

func TestNoEscribeEnCarpetaCompartida(t *testing.T) {
	t.Parallel()

	shared := filepath.Join(t.TempDir(), "compartida")
	if err := os.Mkdir(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "real")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "enlace")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	// Carpeta que es un enlace: se rechaza y no se escribe en el destino.
	c := New(filepath.Join(link, "metrics.json"), 1<<20)
	c.RecordProtocolRequest(context.Background(), "sign", "ok", time.Millisecond)
	if _, err := os.Stat(filepath.Join(target, "metrics.json")); !os.IsNotExist(err) {
		t.Fatalf("se escribió a través de una carpeta enlazada: %v", err)
	}

	// Carpeta del usuario con permisos abiertos: EnsurePrivateDir la cierra
	// antes de escribir, así que el fichero queda solo para el usuario.
	c = New(filepath.Join(shared, "metrics.json"), 1<<20)
	c.RecordProtocolRequest(context.Background(), "sign", "ok", time.Millisecond)
	info, err := os.Stat(shared)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("la carpeta sigue abierta a otros: %o", perm)
	}
}

func TestNoSigueEnlaceEnElFicheroDeMetricas(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victima")
	if err := os.WriteFile(victim, []byte("intacto"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "metrics.json")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}

	c := New(path, 1<<20)
	c.RecordSign(context.Background(), "cades", "ok", time.Millisecond)

	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "intacto" {
		t.Fatalf("se sobrescribió el destino del enlace: %q", data)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("el enlace se sustituyó; se esperaba que no se tocara")
	}
}
