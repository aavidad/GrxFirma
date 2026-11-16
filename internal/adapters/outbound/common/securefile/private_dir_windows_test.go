// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsurePrivateDirWindowsRechazaUnion(t *testing.T) {
	destino := t.TempDir()
	union := filepath.Join(t.TempDir(), "union")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", union, destino).CombinedOutput(); err != nil {
		t.Skipf("no se pudo crear la unión: %v %s", err, out)
	}
	if err := EnsurePrivateDir(union); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("unión admitida: %v", err)
	}
	if entries, _ := os.ReadDir(destino); len(entries) != 0 {
		t.Fatalf("se tocó el destino de la unión: %v", entries)
	}
}

// C:\Windows pertenece a TrustedInstaller: no es del usuario.
func TestCheckPrivateDirWindowsRechazaCarpetaAjena(t *testing.T) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		t.Skip("sin WINDIR")
	}
	if err := CheckPrivateDir(windir); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("carpeta del sistema dada por privada: %v", err)
	}
}

// Una carpeta creada a mano hereda la DACL de su madre: no es privada hasta
// que EnsurePrivateDir le pone la DACL protegida.
func TestCheckPrivateDirWindowsExigeDACLProtegida(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "heredada")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CheckPrivateDir(dir); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("DACL heredada dada por privada: %v", err)
	}
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := CheckPrivateDir(dir); err != nil {
		t.Fatalf("DACL de EnsurePrivateDir no aceptada: %v", err)
	}
	// Abrirla a Todos la deja fuera.
	if out, err := exec.Command("icacls", dir, "/grant", "*S-1-1-0:(R)").CombinedOutput(); err != nil {
		t.Skipf("icacls no disponible: %v %s", err, out)
	}
	if err := CheckPrivateDir(dir); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("carpeta abierta a Todos dada por privada: %v", err)
	}
}
