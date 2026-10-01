//go:build linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nssstore

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Reproduce la lectura concreta que hacía KeyFor durante la comprobación
// online. El enlace de un perfil de usuario debe seguir rechazándose: la
// acción IPC ahora usa el DER público del catálogo y no exporta esa clave.
func TestClonarBaseNSSTemporal_EnlaceUsuarioDevuelveELOOP(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cert9.db")
	if err := os.WriteFile(file, []byte("NSS sintético"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(dir, "key4.db")); err != nil {
		t.Fatal(err)
	}
	_, err := clonarBaseNSSTemporal(dir)
	if !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("clonarBaseNSSTemporal() = %v; se esperaba ELOOP", err)
	}
}
