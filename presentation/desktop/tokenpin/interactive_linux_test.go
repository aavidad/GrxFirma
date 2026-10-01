// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build pinentry_ui_qa

package tokenpin

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
)

// Opt-in physical UI test: use a private Xvfb display, not a personal session.
// Only the synthetic PIN below is expected; no certificate/key is accessed.
func TestPinentryNativeDialog(t *testing.T) {
	if os.Getenv("GRXFIRMA_TEST_PINENTRY_UI") != "isolated-synthetic" {
		t.Skip("requires explicit isolated synthetic UI harness")
	}
	scenario := os.Getenv("GRXFIRMA_TEST_PINENTRY_ACTION")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	mode := pkcs11worker.PINMode{ProtectedAuthenticationPath: scenario == "pinpad"}
	pin, err := New("es").Request(ctx, domain.CertificateRef{ID: "synthetic-ui", Fingerprint: strings.Repeat("a", 64)}, mode)
	defer clear(pin)
	if scenario == "cancel" {
		if !errors.Is(err, pkcs11worker.ErrPINCancelled) || pin != nil {
			t.Fatalf("cancellation failed: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if mode.ProtectedAuthenticationPath {
		if len(pin) != 0 {
			t.Fatal("pinpad returned a secret")
		}
		return
	}
	if string(pin) != "1234%" {
		t.Fatal("unexpected synthetic PIN")
	}
}
