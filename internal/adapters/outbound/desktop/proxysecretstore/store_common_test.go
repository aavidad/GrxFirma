// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeSecretID(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{
		"0123456789abcdef0123456789abcdef",
		"proxy-secret_1.legacy",
	} {
		got, err := normalizeSecretID(valid)
		if err != nil || got != valid {
			t.Fatalf("normalizeSecretID(%q) = %q, %v", valid, got, err)
		}
	}

	for _, invalid := range []string{
		"",
		".",
		"..",
		"../otro",
		`..\otro`,
		"unidad:C",
		"con espacio",
		"secréto",
		"CON",
		"lpt1.legacy",
		strings.Repeat("a", maxProxySecretIDBytes+1),
	} {
		if _, err := normalizeSecretID(invalid); !errors.Is(err, ErrInvalidSecretID) {
			t.Fatalf("normalizeSecretID(%q) error = %v, want ErrInvalidSecretID", invalid, err)
		}
	}
}
