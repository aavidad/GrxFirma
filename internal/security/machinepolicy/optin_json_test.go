//go:build !windows && !android

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package machinepolicy

import "testing"

func TestOptInJSONRequiresBooleanObject(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"prohibir_cifrado_cms_heredado":null}`, `{"prohibir_cifrado_cms_heredado":"false"}`} {
		if _, _, err := decodeOptIn([]byte(raw), ProhibirCifradoCMSHeredado); err == nil {
			t.Fatalf("directiva ilegible aceptada: %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"prohibir_cifrado_cms_heredado":false}`, `{"prohibir_cifrado_cms_heredado":true}`} {
		if _, _, err := decodeOptIn([]byte(raw), ProhibirCifradoCMSHeredado); err != nil {
			t.Fatalf("directiva válida: %s: %v", raw, err)
		}
	}
}
