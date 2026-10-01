// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ioskeychain

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/domain"
)

func certPrueba() domain.CertificateRef {
	return domain.CertificateRef{
		ID:          "cert-ios-1",
		Fingerprint: "11:22:33",
	}
}

func TestProviderKeyFor_Exito(t *testing.T) {
	t.Parallel()

	provider := Nuevo(ResolveFunc(func(context.Context, domain.CertificateRef) (ResolvedKey, error) {
		return ResolvedKey{
			KeyID:         "ios-key-1",
			KeychainTag:   "es.dipgra.grxfirma.key",
			SecureEnclave: true,
		}, nil
	}))

	key, err := provider.KeyFor(context.Background(), certPrueba())
	if err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	if key.KeyID() != "ios-key-1" {
		t.Fatalf("KeyID = %q, want ios-key-1", key.KeyID())
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
	if key.KeyID() != "cert-ios-1" {
		t.Fatalf("KeyID = %q, want cert-ios-1", key.KeyID())
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
		return ResolvedKey{}, errors.New("keychain inaccesible")
	}))

	_, err := provider.KeyFor(context.Background(), certPrueba())
	if err == nil {
		t.Fatal("KeyFor() error = nil, want error")
	}
}
