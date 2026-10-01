// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && !amd64 && !arm64

package pkcs11worker

import "testing"

func TestDriverSeccompUnsupportedArchitectureFailsClosed(t *testing.T) {
	// The unsupported stub never changes process state or silently succeeds.
	if err := RestrictDriverSyscalls(); err == nil {
		t.Fatal("unsupported native driver architecture silently accepted")
	}
}
