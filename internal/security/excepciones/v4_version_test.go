// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
	"grxfirma/internal/security/machinepolicy"
	"testing"
)

func TestV4PreviousTextNeedsFreshAcceptance(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	r := v3Receipt("tsa_http")
	r.TextVersion = "2026-11-15.1"
	v3Write(t, dir, "settings.json", &r)
	ctx := WithConfigDir(context.Background(), dir)
	if Check("tsa_http", ctx) == nil {
		t.Fatal("el recibo del texto anterior sigue activando")
	}
	var oldState State
	for _, s := range ListAt(dir) {
		if s.ID == "tsa_http" {
			oldState = s
		}
	}
	if oldState.Enabled || oldState.LastAcceptance == nil || oldState.LastAcceptance.TextVersion != r.TextVersion {
		t.Fatal("recibo anterior no se conserva como histórico desactivado")
	}
	block := DescribeAt(dir, "tsa_http")
	if !block.Activable || block.AcceptanceVersion == r.TextVersion {
		t.Fatalf("no ofrece aceptación nueva: %+v", block)
	}
	if _, err := ActivateLocal(ForLocalDesktop(ctx), dir, ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: r.TextVersion}); err != ErrAcceptanceVersion {
		t.Fatalf("versión vieja aceptada: %v", err)
	}
	if _, err := ActivateLocal(ForLocalDesktop(ctx), dir, ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	if err := Check("tsa_http", ctx); err != nil {
		t.Fatal("aceptación nueva no activa", err)
	}
}
