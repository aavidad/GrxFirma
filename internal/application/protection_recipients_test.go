// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type protectionRecipientExchangeMock struct {
	recipient domain.ProtectionRecipient
	data      []byte
	err       error
}

func (m *protectionRecipientExchangeMock) Export(context.Context, string) (domain.ProtectionRecipient, []byte, error) {
	return m.recipient, m.data, m.err
}

func (m *protectionRecipientExchangeMock) Import(context.Context, []byte) (domain.ProtectionRecipient, error) {
	return m.recipient, m.err
}

func TestExportProtectionRecipient_Exito(t *testing.T) {
	uc := application.NuevoExportProtectionRecipientUseCase(&protectionRecipientExchangeMock{
		recipient: domain.ProtectionRecipient{ID: "dest-1", Label: "Destino alto"},
		data:      []byte(`{"ok":true}`),
	})
	result, err := uc.Ejecutar(context.Background(), application.ExportProtectionRecipientCommand{RecipientID: "dest-1"})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if result.Recipient.ID != "dest-1" || len(result.Data) == 0 {
		t.Fatalf("resultado inesperado: %#v", result)
	}
}

func TestImportProtectionRecipient_Exito(t *testing.T) {
	uc := application.NuevoImportProtectionRecipientUseCase(&protectionRecipientExchangeMock{
		recipient: domain.ProtectionRecipient{ID: "dest-2", Label: "Importado"},
	})
	result, err := uc.Ejecutar(context.Background(), application.ImportProtectionRecipientCommand{Data: []byte(`{"ok":true}`)})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if result.Recipient.ID != "dest-2" {
		t.Fatalf("resultado inesperado: %#v", result)
	}
}

func TestImportProtectionRecipient_ErrorVacio(t *testing.T) {
	uc := application.NuevoImportProtectionRecipientUseCase(&protectionRecipientExchangeMock{})
	if _, err := uc.Ejecutar(context.Background(), application.ImportProtectionRecipientCommand{}); err == nil {
		t.Fatal("se esperaba error por datos vacíos")
	}
}

func TestExportProtectionRecipient_ErrorStore(t *testing.T) {
	uc := application.NuevoExportProtectionRecipientUseCase(&protectionRecipientExchangeMock{
		err: errors.New("boom"),
	})
	if _, err := uc.Ejecutar(context.Background(), application.ExportProtectionRecipientCommand{RecipientID: "dest-1"}); err == nil {
		t.Fatal("se esperaba error del store")
	}
}
