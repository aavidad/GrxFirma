// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/ports"
)

type loggerQueFalla struct{}

func (loggerQueFalla) Log(context.Context, ports.Evidence) error {
	return errors.New("disco lleno")
}

// Un fallo al guardar la auditoría se devuelve y deja un aviso, sin nombre ni
// contenido del documento.
func TestAuditUseCase_Registrar_FalloDeRegistradorAvisa(t *testing.T) {
	var salida bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&salida, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	uc := application.NuevoAuditUseCase(relojMock{}, loggerQueFalla{})
	err := uc.Registrar(context.Background(), application.AuditCommand{
		OperationType: "firma",
		DocumentName:  "dni-12345678Z.pdf",
		DocumentData:  []byte("secreto"),
		Success:       true,
	})
	if err == nil {
		t.Fatal("se esperaba el error del registrador")
	}
	texto := salida.String()
	if !strings.Contains(texto, "auditoría") || !strings.Contains(texto, "disco lleno") {
		t.Fatalf("falta el aviso de fallo de auditoría: %q", texto)
	}
	if strings.Contains(texto, "12345678Z") || strings.Contains(texto, "secreto") {
		t.Fatalf("el aviso no debe llevar datos del documento: %q", texto)
	}
}
