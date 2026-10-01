// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package sharesheet

import (
	"context"
	"testing"

	mobileinbound "grxfirma/internal/adapters/inbound/mobile"
	"grxfirma/internal/domain"
)

func TestHandle_XMLCompartidoGeneraXAdES(t *testing.T) {
	adaptador := Adaptador{}

	solicitud, err := adaptador.Handle(context.Background(), "application/xml", []byte("<doc/>"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if solicitud.Formato != domain.FormatXAdES {
		t.Fatalf("formato inesperado: %s", solicitud.Formato)
	}
}

func TestHandle_DescriptorNombrePreservado(t *testing.T) {
	adaptador := Adaptador{}

	solicitud, err := adaptador.Handle(context.Background(), "acuerdo.pdf", []byte("%PDF-1.4\n"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if solicitud.SignCommand.Document.Name != "acuerdo.pdf" {
		t.Fatalf("nombre inesperado: %s", solicitud.SignCommand.Document.Name)
	}
}

func TestHandleMultiple_CreaSolicitudDeLote(t *testing.T) {
	adaptador := Adaptador{}

	solicitud, err := adaptador.HandleMultiple(context.Background(), []mobileinbound.DocumentoCompartidoEntrada{
		{Descriptor: "acuerdo.pdf", Payload: []byte("%PDF-1.4\n")},
		{Descriptor: "factura.xml", Payload: []byte("<doc/>")},
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.BatchCommand == nil {
		t.Fatal("se esperaba BatchCommand")
	}
	if len(solicitud.BatchCommand.Jobs) != 2 {
		t.Fatalf("jobs inesperados: %d", len(solicitud.BatchCommand.Jobs))
	}
	if solicitud.BatchCommand.Jobs[0].Document.Name != "acuerdo.pdf" {
		t.Fatalf("nombre inesperado en job 0: %s", solicitud.BatchCommand.Jobs[0].Document.Name)
	}
	if solicitud.BatchCommand.Jobs[1].Document.Name != "factura.xml" {
		t.Fatalf("nombre inesperado en job 1: %s", solicitud.BatchCommand.Jobs[1].Document.Name)
	}
}
