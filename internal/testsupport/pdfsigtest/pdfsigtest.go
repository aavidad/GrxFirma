// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pdfsigtest aísla la base NSS que necesitan las pruebas con pdfsig.
package pdfsigtest

import (
	"os/exec"
	"testing"
)

// Command prepara una base NSS vacía para esta prueba y devuelve pdfsig con
// ella. Solo la comprobación externa se omite cuando falta certutil.
func Command(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	if _, err := exec.LookPath("certutil"); err != nil {
		t.Skip("se omite la comprobación externa con pdfsig: certutil no está disponible para crear la base NSS")
	}
	dir := t.TempDir()
	if out, err := exec.Command("certutil", "-N", "-d", "sql:"+dir, "--empty-password").CombinedOutput(); err != nil {
		t.Fatalf("no se pudo crear la base NSS temporal para pdfsig: %v\n%s", err, out)
	}
	return exec.Command("pdfsig", append([]string{"-nssdir", dir}, args...)...)
}
