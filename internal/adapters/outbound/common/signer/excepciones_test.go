// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"testing"

	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func activarExcepcionPrueba(t *testing.T, id string) {
	t.Helper()
	e, ok := excepciones.Find(id)
	if !ok {
		t.Fatalf("excepción desconocida: %s", id)
	}
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	excepciones.SetForTest(t, id, true)
}

func avisoExcepcionPrueba(id string) string { return excepciones.Warning(id) }
