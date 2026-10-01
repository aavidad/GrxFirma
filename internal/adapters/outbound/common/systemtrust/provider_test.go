// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package systemtrust

import (
	"context"
	"testing"
)

func TestProviderSolicitaRaicesDelSistema(t *testing.T) {
	anchors, err := New().Anchors(context.Background())
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}
	if anchors.IsEmpty() || !anchors.UseSystemRoots {
		t.Fatalf("anclas inesperadas: %+v", anchors)
	}
}

func TestProviderRespetaContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Anchors(ctx); err == nil {
		t.Fatal("Anchors() debía propagar la cancelación")
	}
}
