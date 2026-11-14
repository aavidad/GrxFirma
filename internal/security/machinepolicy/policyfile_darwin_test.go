// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package machinepolicy

import (
	"strings"
	"testing"
)

// F-07: en macOS /etc es un enlace a /private/etc y la lectura no sigue
// enlaces; la ruta de la política no puede pasar por él.
func TestPolicyFileDarwinNoPasaPorEnlaceEtc(t *testing.T) {
	if strings.HasPrefix(PolicyFile, "/etc/") {
		t.Fatalf("PolicyFile = %q atraviesa el enlace /etc", PolicyFile)
	}
}
