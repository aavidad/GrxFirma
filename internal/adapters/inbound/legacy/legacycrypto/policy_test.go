// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package legacycrypto

import (
	"errors"
	"testing"

	"grxfirma/internal/security/machinepolicy"
)

func TestDESDesactivadoPorDefecto(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	if DESEnabled() {
		t.Fatal("DES no debe estar habilitado por defecto")
	}
	if err := RequireDES(); !errors.Is(err, ErrDESDisabled) {
		t.Fatalf("RequireDES() error = %v", err)
	}
}

func TestDESRequiereOptInExplicito(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	if !DESEnabled() {
		t.Fatal("la política de máquina no habilitó DES")
	}
	if err := RequireDES(); err != nil {
		t.Fatalf("RequireDES() error = %v", err)
	}
}

// Una variable de entorno (que una web o guía podría pedir definir al
// usuario) ya no basta para rebajar la seguridad.
func TestDESNoSeHabilitaPorVariableDeEntorno(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	for _, value := range []string{"1", "true", "yes", "si", "on"} {
		t.Setenv(EnvEnableLegacyDES, value)
		if DESEnabled() {
			t.Fatalf("%s=%q no debe habilitar DES", EnvEnableLegacyDES, value)
		}
	}
}
