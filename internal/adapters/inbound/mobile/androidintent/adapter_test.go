// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package androidintent

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	mobileinbound "grxfirma/internal/adapters/inbound/mobile"
	"grxfirma/internal/domain"
)

func TestHandle_ViewLegacyAfirmaURI(t *testing.T) {
	adaptador := New(afirmauri.New(trustStub{estado: domain.TrustAllowed}))

	solicitud, err := adaptador.Handle(context.Background(),
		"android.intent.action.VIEW",
		[]byte("afirma://sign?id=req-1&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat=QUJD"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if solicitud.Origen != "android-intent" {
		t.Fatalf("origen inesperado: %s", solicitud.Origen)
	}
}

func TestHandle_SendInfierePDFComoPAdES(t *testing.T) {
	adaptador := New(nil)

	solicitud, err := adaptador.Handle(context.Background(), "SEND", []byte("%PDF-1.4\n"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if solicitud.Formato != domain.FormatPAdES {
		t.Fatalf("formato inesperado: %s", solicitud.Formato)
	}
	if solicitud.SignCommand.Document.Name != "documento.pdf" {
		t.Fatalf("nombre inesperado: %s", solicitud.SignCommand.Document.Name)
	}
}

func TestHandleShared_SendPreservaDescriptorAndroid(t *testing.T) {
	adaptador := New(nil)

	solicitud, err := adaptador.HandleShared(context.Background(), "android.intent.action.SEND", "contrato-final.pdf", []byte("%PDF-1.4\n"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if solicitud.SignCommand.Document.Name != "contrato-final.pdf" {
		t.Fatalf("nombre inesperado: %s", solicitud.SignCommand.Document.Name)
	}
	if solicitud.Formato != domain.FormatPAdES {
		t.Fatalf("formato inesperado: %s", solicitud.Formato)
	}
}

func TestHandleSharedMultiple_SendMultipleGeneraLote(t *testing.T) {
	adaptador := New(nil)

	solicitud, err := adaptador.HandleSharedMultiple(context.Background(), "android.intent.action.SEND_MULTIPLE", []mobileinbound.DocumentoCompartidoEntrada{
		{Descriptor: "contrato.pdf", Payload: []byte("%PDF-1.4\n")},
		{Descriptor: "factura.xml", Payload: []byte("<root/>")},
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.BatchCommand == nil {
		t.Fatal("se esperaba BatchCommand")
	}
	if solicitud.Tipo != mobileinbound.TipoFirmaEnLote {
		t.Fatalf("tipo inesperado: %s", solicitud.Tipo)
	}
	if len(solicitud.BatchCommand.Jobs) != 2 {
		t.Fatalf("jobs inesperados: %d", len(solicitud.BatchCommand.Jobs))
	}
	if solicitud.BatchCommand.Jobs[0].Document.Name != "contrato.pdf" {
		t.Fatalf("nombre primer job inesperado: %s", solicitud.BatchCommand.Jobs[0].Document.Name)
	}
	if solicitud.BatchCommand.Jobs[1].Document.Name != "factura.xml" {
		t.Fatalf("nombre segundo job inesperado: %s", solicitud.BatchCommand.Jobs[1].Document.Name)
	}
}

func TestHandleSharedMultiple_RechazaAccionNoMultiple(t *testing.T) {
	adaptador := New(nil)

	_, err := adaptador.HandleSharedMultiple(context.Background(), "android.intent.action.SEND", []mobileinbound.DocumentoCompartidoEntrada{
		{Descriptor: "contrato.pdf", Payload: []byte("%PDF-1.4\n")},
	})
	if err == nil {
		t.Fatal("se esperaba error por accion no multiple")
	}
}

type trustStub struct {
	estado domain.TrustStatus
}

func (t trustStub) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	return domain.TrustDecision{Origin: origin, Status: t.estado}, nil
}
func (t trustStub) Allow(context.Context, string) error  { return nil }
func (t trustStub) Deny(context.Context, string) error   { return nil }
func (t trustStub) Remove(context.Context, string) error { return nil }
