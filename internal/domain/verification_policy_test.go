// SPDX-License-Identifier: EUPL-1.2
package domain_test

import (
	"encoding/json"
	"errors"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"testing"
)

func TestVerificationPolicyErrorTypedAndNotSerializable(t *testing.T) {
	blocked := &excepciones.Block{ID: "sha1_leer", BlockKey: "excepciones.sha1_leer.bloqueo"}
	v := domain.NewVerificationSuccess("XAdES", "texto [sha1_leer; excepciones.sha1_leer.bloqueo]", nil).WithPolicyError(blocked).Normalize()
	var found *excepciones.Block
	if v.Valid || !errors.As(v.PolicyError(), &found) || found.ID != "sha1_leer" {
		t.Fatal("se pierde el bloqueo tipado", v)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded domain.VerificationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PolicyError() != nil {
		t.Fatal("JSON fabrica el bloqueo")
	}
	if err := json.Unmarshal([]byte(`{"Reason":"[sha1_leer; excepciones.sha1_leer.bloqueo]","policyError":{"ID":"sha1_leer"}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PolicyError() != nil {
		t.Fatal("texto o campo externo fabrica el bloqueo")
	}
}
