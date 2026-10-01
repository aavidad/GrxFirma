// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cryptopolicy

import (
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/security/machinepolicy"
)

func TestSHA1DesactivadoPorDefecto(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	if LegacySHA1Enabled() {
		t.Fatal("SHA-1 de firma no debe estar habilitado por defecto")
	}
	if err := RequireLegacySHA1(); !errors.Is(err, ErrSHA1Disabled) {
		t.Fatalf("RequireLegacySHA1() error = %v", err)
	}
	message := ErrSHA1Disabled.Error()
	for _, fragment := range []string{"SHA-1", "inseguro", "ninguna firma", "portal", "SHA-256"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("el rechazo no explica el riesgo o la remediación (%q): %q", fragment, message)
		}
	}
	if strings.Contains(message, EnvEnableLegacySHA1) {
		t.Fatalf("el mensaje visible no debe recomendar al usuario habilitar SHA-1: %q", message)
	}
}

func TestSHA1RequiereOptInExplicito(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, true)
	if err := RequireLegacySHA1(); err != nil {
		t.Fatalf("la política de máquina no habilitó SHA-1: %v", err)
	}
}

func TestSHA1NoSeHabilitaPorVariableDeEntorno(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	for _, value := range []string{"1", "true", "yes", "si", "on"} {
		t.Setenv(EnvEnableLegacySHA1, value)
		if err := RequireLegacySHA1(); err == nil {
			t.Fatalf("%s=%q no debe habilitar SHA-1", EnvEnableLegacySHA1, value)
		}
	}
}
