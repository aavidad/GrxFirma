// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobiledocumentpicker

import (
	"context"
	"errors"
	"testing"
)

func TestPickerPick_Exito(t *testing.T) {
	t.Parallel()

	picker := Nuevo(ResolveFunc(func(context.Context) (SelectedDocument, error) {
		return SelectedDocument{
			Name:     "contrato.pdf",
			Content:  []byte("PDF"),
			MIMEType: "application/pdf",
		}, nil
	}))

	doc, err := picker.Pick(context.Background())
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if doc.Name != "contrato.pdf" {
		t.Fatalf("Name = %q, want contrato.pdf", doc.Name)
	}
	if doc.MIMEType != "application/pdf" {
		t.Fatalf("MIMEType = %q, want application/pdf", doc.MIMEType)
	}
}

func TestPickerPick_ConfiguraValoresPorDefecto(t *testing.T) {
	t.Parallel()

	picker := Nuevo(ResolveFunc(func(context.Context) (SelectedDocument, error) {
		return SelectedDocument{
			Content: []byte("BIN"),
		}, nil
	}))

	doc, err := picker.Pick(context.Background())
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if doc.Name != "documento" {
		t.Fatalf("Name = %q, want documento", doc.Name)
	}
	if doc.MIMEType != "application/octet-stream" {
		t.Fatalf("MIMEType = %q, want application/octet-stream", doc.MIMEType)
	}
}

func TestPickerPick_SinResolver(t *testing.T) {
	t.Parallel()

	_, err := Picker{}.Pick(context.Background())
	if err == nil {
		t.Fatal("Pick() error = nil, want error")
	}
}

func TestPickerPick_PropagaErrorResolver(t *testing.T) {
	t.Parallel()

	picker := Nuevo(ResolveFunc(func(context.Context) (SelectedDocument, error) {
		return SelectedDocument{}, errors.New("cancelado")
	}))

	_, err := picker.Pick(context.Background())
	if err == nil {
		t.Fatal("Pick() error = nil, want error")
	}
}
