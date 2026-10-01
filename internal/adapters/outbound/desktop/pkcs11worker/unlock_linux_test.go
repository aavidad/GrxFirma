// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestUnlockMetadataWireContract(t *testing.T) {
	ref := domain.CertificateRef{ID: "synthetic", Fingerprint: strings.Repeat("a", 64), SigningKeyNeedsUnlock: true}
	public, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(public)), "unlock") {
		t.Fatal("private catalog evidence leaked through public ref JSON")
	}
	entry := CatalogEntry{Certificate: ref, SigningKeyNeedsUnlock: true}
	wire, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CatalogEntry
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.SigningKeyNeedsUnlock || decoded.HasSigningKey || decoded.Certificate.SigningKeyNeedsUnlock {
		t.Fatal("unlock state not transported exclusively by wrapper")
	}
	r := Response{Code: "ok", Version: ProtocolVersion, ID: strings.Repeat("b", 32), Certificates: []CatalogEntry{decoded}}
	request := Request{ID: r.ID, Operation: "list"}
	if err := validateResponse(request, r); err != nil {
		t.Fatal(err)
	}
	r.Certificates[0].HasSigningKey = true
	if err := validateResponse(request, r); !errors.Is(err, ErrProtocol) {
		t.Fatal("contradictory evidence accepted")
	}
}
