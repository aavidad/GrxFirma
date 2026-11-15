// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"errors"
	"os"
	"testing"
)

// Sin carpeta indicada, el TLS local no se genera en la carpeta temporal.
func TestTLSLocalSinCarpetaNoUsaLaTemporal(t *testing.T) {
	temporal := t.TempDir()
	t.Setenv("TMPDIR", temporal)
	t.Setenv("TMP", temporal)
	t.Setenv("TEMP", temporal)
	if _, _, err := EnsureLocalhostCertificate("", "prueba"); !errors.Is(err, ErrDirectorioTLSNoIndicado) {
		t.Fatalf("EnsureLocalhostCertificate sin carpeta = %v", err)
	}
	if _, _, _, err := EnsureLocalhostCertificateWithLocalCA(" ", "prueba"); !errors.Is(err, ErrDirectorioTLSNoIndicado) {
		t.Fatalf("EnsureLocalhostCertificateWithLocalCA sin carpeta = %v", err)
	}
	if entradas, _ := os.ReadDir(temporal); len(entradas) != 0 {
		t.Fatalf("se escribió en la carpeta temporal: %v", entradas)
	}
}

// Sin carpeta personal válida, el servidor REST no sitúa su TLS en la
// carpeta temporal y el diagnóstico lo da por no disponible.
func TestTLSRestSinCarpetaPersonalFallaCerrado(t *testing.T) {
	for _, variable := range []string{"HOME", "USERPROFILE", "APPDATA"} {
		t.Setenv(variable, "")
	}
	a := &Adaptador{}
	if dir, err := a.directorioTLSRest(); err == nil {
		t.Fatalf("directorioTLSRest sin carpeta personal = %q", dir)
	}
	if got := a.tlsStoreDiagnostic().State; got != "unavailable" {
		t.Fatalf("diagnóstico sin carpeta personal = %q", got)
	}
	explicito := &Adaptador{ConfigDir: t.TempDir()}
	if _, err := explicito.directorioTLSRest(); err != nil {
		t.Fatalf("con carpeta explícita = %v", err)
	}
}
