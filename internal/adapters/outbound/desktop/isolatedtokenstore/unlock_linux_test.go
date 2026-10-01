// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

func TestUnlockMetadataDoesNotInventKeyEvidence(t *testing.T) {
	f := newFixture(t, "EC", nil)
	for _, unlock := range []bool{false, true} {
		f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) {
			ref := f.ref
			ref.SigningKeyNeedsUnlock = !unlock
			return pkcs11worker.Response{Code: "ok", Certificates: []pkcs11worker.CatalogEntry{{Certificate: ref, SigningKeyNeedsUnlock: unlock}}}, nil
		}
		refs, err := f.store.List(context.Background())
		if err != nil || len(refs) != 1 || refs[0].HasSigningKey || refs[0].SigningKeyNeedsUnlock != unlock {
			t.Fatalf("refs=%+v err=%v", refs, err)
		}
	}
	f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) {
		return pkcs11worker.Response{Code: "ok", Certificates: []pkcs11worker.CatalogEntry{{Certificate: f.ref, HasSigningKey: true, SigningKeyNeedsUnlock: true}}}, nil
	}
	if _, err := f.store.List(context.Background()); !errors.Is(err, ErrInvalidResponse) {
		t.Fatal("contradictory evidence accepted")
	}
}
