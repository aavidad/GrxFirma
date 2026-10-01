// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobile

import "testing"

func TestBuildSolicitudDocumento_SobrescribePayloadOriginalYLimpiaInterno(t *testing.T) {
	payload := []byte("documento muy sensible")

	solicitud, err := BuildSolicitudDocumento("test", "demo.pdf", "application/pdf", "PAdES", "sign", payload)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	for _, b := range payload {
		if b != 0 {
			t.Fatal("el payload original no fue sobrescrito")
		}
	}

	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if len(solicitud.SignCommand.Document.Content) == 0 {
		t.Fatal("se esperaba copia interna del documento")
	}

	solicitud.Liberar()
	for _, b := range solicitud.SignCommand.Document.Content {
		if b != 0 {
			t.Fatal("el buffer interno no fue limpiado")
		}
	}
}

func TestBuildSolicitudDocumentosCompartidos_CreaLoteYLimpiaPayloads(t *testing.T) {
	payloadA := []byte("%PDF-1.4\n")
	payloadB := []byte("<root/>")

	solicitud, err := BuildSolicitudDocumentosCompartidos("android-intent", []DocumentoCompartidoEntrada{
		{Descriptor: "contrato.pdf", Payload: payloadA},
		{Descriptor: "factura.xml", Payload: payloadB},
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.BatchCommand == nil {
		t.Fatal("se esperaba BatchCommand")
	}
	if solicitud.Tipo != TipoFirmaEnLote {
		t.Fatalf("tipo inesperado: %s", solicitud.Tipo)
	}
	if len(solicitud.BatchCommand.Jobs) != 2 {
		t.Fatalf("jobs inesperados: %d", len(solicitud.BatchCommand.Jobs))
	}
	for _, b := range payloadA {
		if b != 0 {
			t.Fatal("payloadA original no fue sobrescrito")
		}
	}
	for _, b := range payloadB {
		if b != 0 {
			t.Fatal("payloadB original no fue sobrescrito")
		}
	}

	solicitud.Liberar()
	for i := range solicitud.BatchCommand.Jobs {
		for _, b := range solicitud.BatchCommand.Jobs[i].Document.Content {
			if b != 0 {
				t.Fatalf("buffer interno del job %d no fue limpiado", i)
			}
		}
	}
}
