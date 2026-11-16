// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePrivateDirCierraPermisosDeGrupoYOtros(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "abierta")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := CheckPrivateDir(dir); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("carpeta 0777 dada por privada: %v", err)
	}
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("permisos %o, se esperaba 700", info.Mode().Perm())
	}
}

func TestPrivateDirRechazaCarpetaDeOtroUsuario(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root es propietario de todo")
	}
	// / pertenece a root.
	if err := CheckPrivateDir("/"); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("carpeta ajena admitida al comprobar: %v", err)
	}
	if err := EnsurePrivateDir("/"); !errors.Is(err, ErrNotPrivateDir) {
		t.Fatalf("carpeta ajena admitida al crear: %v", err)
	}
}
