// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package androidkeystore

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/domain"
)

func certPrueba() domain.CertificateRef {
	return domain.CertificateRef{
		ID:          "cert-android-1",
		Fingerprint: "aa:bb:cc",
	}
}

func TestProviderKeyFor_Exito(t *testing.T) {
	t.Parallel()

	provider := Nuevo(ResolveFunc(func(context.Context, domain.CertificateRef) (ResolvedKey, error) {
		return ResolvedKey{
			KeyID: "android-key-1",
			Alias: "dipgra.signing",
		}, nil
	}))

	key, err := provider.KeyFor(context.Background(), certPrueba())
	if err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	if key.KeyID() != "android-key-1" {
		t.Fatalf("KeyID = %q, want android-key-1", key.KeyID())
	}
}

func TestProviderKeyFor_FallbackACertificateID(t *testing.T) {
	t.Parallel()

	provider := Nuevo(ResolveFunc(func(context.Context, domain.CertificateRef) (ResolvedKey, error) {
		return ResolvedKey{}, nil
	}))

	key, err := provider.KeyFor(context.Background(), certPrueba())
	if err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	if key.KeyID() != "cert-android-1" {
		t.Fatalf("KeyID = %q, want cert-android-1", key.KeyID())
	}
}

func TestProviderKeyFor_SinResolver(t *testing.T) {
	t.Parallel()

	_, err := Provider{}.KeyFor(context.Background(), certPrueba())
	if err == nil {
		t.Fatal("KeyFor() error = nil, want error")
	}
}

func TestProviderKeyFor_PropagaErrorResolver(t *testing.T) {
	t.Parallel()

	provider := Nuevo(ResolveFunc(func(context.Context, domain.CertificateRef) (ResolvedKey, error) {
		return ResolvedKey{}, errors.New("alias no encontrado")
	}))

	_, err := provider.KeyFor(context.Background(), certPrueba())
	if err == nil {
		t.Fatal("KeyFor() error = nil, want error")
	}
}
