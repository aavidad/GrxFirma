// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"grxfirma/internal/domain"
	"testing"
	"time"
)

func TestTokenMetadataDoesNotClaimUnlockedKey(t *testing.T) {
	now := time.Now()
	items := (&Manejador{}).certsAJSON([]domain.CertificateRef{
		{ID: "locked", SigningKeyNeedsUnlock: true, NotAfter: now.Add(time.Hour)},
		{ID: "expired", SigningKeyNeedsUnlock: true, NotAfter: now.Add(-time.Hour)},
		{ID: "unknown", SigningKeyNeedsUnlock: true},
		{ID: "missing", NotAfter: now.Add(time.Hour)},
		{ID: "available", HasSigningKey: true, NotAfter: now.Add(time.Hour)},
	})
	if items[0].CanSign || !items[0].NeedsUnlock || items[0].Status != "Requiere autorización de la tarjeta" {
		t.Fatal("locked key misrepresented")
	}
	for _, item := range items[1:4] {
		if item.CanSign || item.NeedsUnlock {
			t.Fatalf("unusable certificate %s offered", item.ID)
		}
	}
	if !items[4].CanSign || items[4].NeedsUnlock {
		t.Fatal("available key changed")
	}
}
