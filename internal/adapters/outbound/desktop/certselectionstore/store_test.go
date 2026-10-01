// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certselectionstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_GuardaYCargaPreferencias(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	store.sessionPath = filepath.Join(t.TempDir(), "session.json")

	if err := store.SaveSession(context.Background(), "https://portal.example", "cert-sesion"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.SavePersistent(context.Background(), "https://portal.example", "cert-persistente"); err != nil {
		t.Fatalf("SavePersistent: %v", err)
	}

	idSesion, ok, err := store.LoadSession(context.Background(), "https://portal.example")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if !ok || idSesion != "cert-sesion" {
		t.Fatalf("preferencia de sesión inesperada ok=%v id=%q", ok, idSesion)
	}

	idPersistente, ok, err := store.LoadPersistent(context.Background(), "https://portal.example")
	if err != nil {
		t.Fatalf("LoadPersistent: %v", err)
	}
	if !ok || idPersistente != "cert-persistente" {
		t.Fatalf("preferencia persistente inesperada ok=%v id=%q", ok, idPersistente)
	}
}

func TestStore_SavePersistentRechazaDestinoSymlink(t *testing.T) {
	cfg := t.TempDir()
	store := New(cfg)
	victim := filepath.Join(t.TempDir(), "victim.json")
	if err := os.WriteFile(victim, []byte(`{"safe":"unchanged"}`), 0o600); err != nil {
		t.Fatalf("WriteFile victim: %v", err)
	}
	path := filepath.Join(cfg, "preferred-certificates.json")
	if err := os.Symlink(victim, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := store.SavePersistent(context.Background(), "https://portal.example", "cert-id"); err == nil {
		t.Fatal("SavePersistent accepted a symlink destination")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("ReadFile victim: %v", err)
	}
	if string(got) != `{"safe":"unchanged"}` {
		t.Fatalf("victim changed to %q", got)
	}
}
