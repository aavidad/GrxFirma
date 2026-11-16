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
