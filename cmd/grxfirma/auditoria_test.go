// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"os"
	"testing"

	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// H-02: la composición de la CLI y del REST guarda la auditoría de verdad.
func TestConstruirServiciosGuardaAuditoria(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("XDG_DATA_HOME", home)

	servicios, err := construirServicios("", "", "", "")
	if err != nil {
		t.Fatalf("construirServicios: %v", err)
	}
	doc, err := domain.NewDocument("prueba.pdf", []byte("%PDF-1.7\nno firmado"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = servicios.verificar.Execute(context.Background(), application.VerifyCommand{SignedDocument: doc})

	ruta, err := auditlog.RutaPorDefecto()
	if err != nil {
		t.Fatal(err)
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("la operación no dejó registro de auditoría en %s: %v", ruta, err)
	}
	if len(datos) == 0 {
		t.Fatal("el registro de auditoría está vacío")
	}
}
