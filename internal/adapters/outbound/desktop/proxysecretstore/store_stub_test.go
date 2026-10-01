// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !windows && !darwin

package proxysecretstore

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/ports"
)

func TestStore_StubNoDisponibleEnEstaPlataforma(t *testing.T) {
	t.Parallel()

	store := New()
	if _, err := store.Store(context.Background(), "corp-proxy", ports.ProxySecretMaterial{
		Username: "alberto",
		Password: []byte("secreto"),
	}); !errors.Is(err, ErrNoDisponibleEnEstaPlataforma) {
		t.Fatalf("Store() error = %v, want ErrNoDisponibleEnEstaPlataforma", err)
	}
	if _, err := store.Load(context.Background(), "id"); !errors.Is(err, ErrNoDisponibleEnEstaPlataforma) {
		t.Fatalf("Load() error = %v, want ErrNoDisponibleEnEstaPlataforma", err)
	}
	if err := store.Delete(context.Background(), "id"); !errors.Is(err, ErrNoDisponibleEnEstaPlataforma) {
		t.Fatalf("Delete() error = %v, want ErrNoDisponibleEnEstaPlataforma", err)
	}
	status, err := store.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Available {
		t.Fatalf("status inesperado: %#v", status)
	}
	wantBackend, wantReason := backendReadinessForPlatform(status.Platform)
	if status.Backend != wantBackend {
		t.Fatalf("Status().Backend = %q, want %q", status.Backend, wantBackend)
	}
	if status.Reason != wantReason {
		t.Fatalf("Status().Reason = %q, want %q", status.Reason, wantReason)
	}
}
