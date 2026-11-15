// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/testsupport/perfil"
)

func TestWriteFileNoSustituyeSinConfirmacionYLeePorDescriptor(t *testing.T) {
	home := perfil.Nuevo(t)
	dir := filepath.Join(home, "Descargas")
	first := filepath.Join(dir, "firma.pdf")
	if got, err := WriteFile(first, []byte("primera"), false); err != nil || got != first {
		t.Fatalf("primer guardado: %q, %v", got, err)
	}
	second, err := WriteFile(first, []byte("segunda"), false)
	if err != nil || second == first || filepath.Base(second) != "firma_001.pdf" {
		t.Fatalf("guardado sin reemplazar: %q, %v", second, err)
	}
	if data, err := ReadFileLimit(first, 32); err != nil || string(data) != "primera" {
		t.Fatalf("se alteró la primera firma: %q, %v", data, err)
	}
	if data, err := ReadFileLimit(second, 32); err != nil || string(data) != "segunda" {
		t.Fatalf("segunda firma: %q, %v", data, err)
	}
	if got, err := WriteFile(first, []byte("confirmada"), true); err != nil || got != first {
		t.Fatalf("reemplazo confirmado: %q, %v", got, err)
	}
	if data, err := ReadFileLimit(first, 32); err != nil || string(data) != "confirmada" {
		t.Fatalf("contenido reemplazado: %q, %v", data, err)
	}
}

func TestWriteFileNoSigueEnlaceEnDirectorioIntermedio(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("apertura fijada por descriptor Linux")
	}
	home := perfil.Nuevo(t)
	config := filepath.Join(home, ".config", "grxfirma")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "Descargas")
	if err := os.Symlink(config, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFile(filepath.Join(alias, "settings.json"), []byte("ataque"), true); err == nil {
		t.Fatal("se siguió enlace a configuración")
	}
	if _, err := os.Stat(filepath.Join(config, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("la configuración cambió: %v", err)
	}
}

func TestWriteFileAdmiteAliasDelHomeConocido(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("alias de HOME en Linux")
	}
	base := t.TempDir()
	realHome := filepath.Join(base, "home-real")
	if err := os.MkdirAll(realHome, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "home-alias")
	if err := os.Symlink(realHome, alias); err != nil {
		t.Fatal(err)
	}
	perfil.Aislar(t, alias)
	target := filepath.Join(alias, "Descargas", "firma.pdf")
	got, err := WriteFile(target, []byte("firma"), false)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := ReadFileLimit(got, 16); err != nil || string(data) != "firma" {
		t.Fatalf("alias HOME legítimo: %q, %v", data, err)
	}
}

// Un directorio simbólico dentro de la carpeta elegida no se sigue al guardar,
// aunque apunte a una carpeta que no está protegida: antes del lote 30w, fuera
// de Linux se escribía en la ruta física ya resuelta y el enlace no se veía.
func TestWriteFileRechazaDirectorioSimbolicoEnTodosLosSistemas(t *testing.T) {
	home := perfil.Nuevo(t)
	outside := t.TempDir()
	link := filepath.Join(home, "Descargas")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("el entorno no permite crear enlaces simbólicos: %v", err)
	}
	for _, force := range []bool{false, true} {
		if got, err := WriteFile(filepath.Join(link, "escape.txt"), []byte("contenido"), force); err == nil {
			t.Fatalf("force=%v: se guardó a través del enlace en %q", force, got)
		}
		if _, err := os.Lstat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(err) {
			t.Fatalf("force=%v: se escribió en el destino del enlace: %v", force, err)
		}
	}
}
