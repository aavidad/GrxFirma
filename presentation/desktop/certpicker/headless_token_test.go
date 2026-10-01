// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"errors"
	"grxfirma/internal/domain"
	"testing"
)

func TestHeadlessDoesNotSilentlySelectLockedTokenOrAlternative(t *testing.T) {
	for _, refs := range [][]domain.CertificateRef{
		{{ID: "locked", SigningKeyNeedsUnlock: true}},
		{{ID: "locked", SigningKeyNeedsUnlock: true}, {ID: "other", HasSigningKey: true}},
	} {
		result, err := NewHeadless().Select(context.Background(), refs)
		if !errors.Is(err, ErrSeleccionInteractivaNecesaria) || result.Certificado.ID != "" {
			t.Fatal("locked token or another identity silently selected")
		}
	}
	result, err := NewHeadless().Select(context.Background(), []domain.CertificateRef{{ID: "regular", HasSigningKey: true}})
	if err != nil || result.Certificado.ID != "regular" {
		t.Fatal("ordinary headless flow changed")
	}
}
