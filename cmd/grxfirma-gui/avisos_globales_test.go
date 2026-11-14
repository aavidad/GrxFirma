// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/security/avisos"
)

// Lote 17b (3d): la lista global de avisos.Registrar no se vaciaba nunca en
// el motor de escritorio, que atiende muchas operaciones y no la enseña (la
// lee afirma://, que es otro proceso). Al construir el servidor, la lista deja
// de guardar: ni se llena ni mezcla avisos de firmas distintas.
func TestConstruirServidorNoGuardaLosAvisosGlobales(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_RUNTIME_DIR", "")
	dir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	avisos.Registrar("Aviso de antes de arrancar", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, _, err := construirServidor(ctx, "", "", filepath.Join(dir, "s.sock"), localizador.Para("es")); err != nil {
		t.Fatalf("construirServidor: %v", err)
	}
	for i := 0; i < 40; i++ {
		avisos.Registrar(fmt.Sprintf("Aviso de la firma %d", i), "motivo")
	}
	if got := avisos.Todos(); len(got) != 0 {
		t.Fatalf("el motor de escritorio no debe guardar avisos globales: %d", len(got))
	}
}
