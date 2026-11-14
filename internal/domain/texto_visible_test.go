// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "testing"

func TestTextoVisibleCambiaControlesYFormato(t *testing.T) {
	for _, caso := range []struct{ entrada, esperado string }{
		{"CN=Juan García Pérez", "CN=Juan García Pérez"},
		{"CN=a\u202eb", "CN=a?b"},
		{"CN=\u2066Ana\u2069 García", "CN=?Ana? García"},
		{"CN=a\u200bb\u200fc\ufeff", "CN=a?b?c?"},
		{"a\x1b[31mb\n\u0085", "a?[31mb??"},
		{"x\ufffdy", "x?y"},
	} {
		if got := TextoVisible(caso.entrada); got != caso.esperado {
			t.Fatalf("TextoVisible(%q) = %q, esperado %q", caso.entrada, got, caso.esperado)
		}
	}
}
