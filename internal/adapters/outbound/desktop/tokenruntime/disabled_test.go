//go:build !linux || !pkcs11_preview || production

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
)

func TestDefaultAndProductionNeverActivate(t *testing.T) {
	if EnabledInBuild() {
		t.Fatal("preview enabled in default/production build")
	}
	for _, path := range []string{"", "relative", "/nonexistent/config", "/dev/zero", "\x00"} {
		r := New(path, func(context.Context, domain.CertificateRef, pkcs11worker.PINMode) ([]byte, error) {
			t.Fatal("inactive runtime requested PIN")
			return nil, nil
		})
		refs, err := r.List(context.Background())
		if err != nil || len(refs) != 0 {
			t.Fatal("inactive catalog not empty")
		}
		if _, err := r.KeyFor(context.Background(), testRef("unseen")); !errors.Is(err, ErrNotApplicable) {
			t.Fatal(err)
		}
		if !errors.Is(r.Diagnostico(), ErrNotApplicable) {
			t.Fatal("configuration was evaluated before build gate")
		}
	}
}
