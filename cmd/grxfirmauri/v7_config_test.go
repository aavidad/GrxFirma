// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"testing"
)

func aceptarExcepcionProtocoloPrueba(t *testing.T, dir, id string) (context.Context, *domain.AvisosOperacion) {
	t.Helper()
	e, ok := excepciones.Find(id)
	if !ok {
		t.Fatal(id)
	}
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), dir, excepciones.ActivationRequest{ID: id, Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	return domain.ConAvisosOperacion(ctx)
}
