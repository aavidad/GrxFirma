// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/certpicker"
)

type certSelectorContador struct {
	elegir   int
	llamadas int
}

func (c *certSelectorContador) Select(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
	c.llamadas++
	return certpicker.ResultadoSeleccion{Certificado: certs[c.elegir]}, nil
}

func certLoteLocal(id string) domain.CertificateRef {
	return domain.CertificateRef{
		ID:       id,
		Subject:  "CN=" + id,
		NotAfter: time.Now().Add(24 * time.Hour),
	}
}

// handlerLoteLocalConSelector arma un handler de lote local con catálogo,
// selector y proveedor de claves de prueba.
func handlerLoteLocalConSelector(certs []domain.CertificateRef, sel *certSelectorContador, captured *application.ProcessBatchCommand) *legacyWebSocketHandler {
	return &legacyWebSocketHandler{
		batchUC: batchExecutorStub{
			cmd: captured,
			result: application.BatchResult{
				Results: []application.SignResult{{Result: domain.SignatureResult{Data: []byte("firma")}}},
				Errores: map[int]error{},
			},
		},
		catalogo: certificateCatalogStub{certs: certs},
		selector: sel,
		keys: keyProviderStub{key: signingKeyStub{
			id:    "k",
			chain: [][]byte{[]byte("cert-der")},
		}},
	}
}

// A-08: el lote local por socket pasa por el selector de certificado, también
// con un único certificado, y con varios usa el elegido.
func TestHandleLocalBatch_PasaPorElSelector(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		nombre string
		certs  []domain.CertificateRef
		elegir int
	}{
		{nombre: "un certificado", certs: []domain.CertificateRef{certLoteLocal("uno")}},
		{nombre: "dos certificados", certs: []domain.CertificateRef{certLoteLocal("uno"), certLoteLocal("dos")}, elegir: 1},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			sel := &certSelectorContador{elegir: tc.elegir}
			var captured application.ProcessBatchCommand
			h := handlerLoteLocalConSelector(tc.certs, sel, &captured)
			resp, err := h.handleLocalBatch(context.Background(), application.ProcessBatchCommand{},
				afirmauri.Solicitud{Operacion: afirmauri.OperacionLote},
				afirmauri.BatchMetadata{IDs: []string{"d1"}, IsJSON: true})
			if err != nil {
				t.Fatalf("handleLocalBatch: %v", err)
			}
			if resp == "" || resp == "CANCEL" {
				t.Fatalf("respuesta inesperada %q", resp)
			}
			if sel.llamadas != 1 {
				t.Fatalf("Select llamado %d veces; se esperaba 1", sel.llamadas)
			}
			if want := tc.certs[tc.elegir].ID; captured.CertificateID != want {
				t.Fatalf("CertificateID = %q; se esperaba %q", captured.CertificateID, want)
			}
		})
	}
}

// A-08: si el usuario cancela el selector no se firma nada.
func TestHandleLocalBatch_SinSelectorNoFirma(t *testing.T) {
	t.Parallel()

	var captured application.ProcessBatchCommand
	h := &legacyWebSocketHandler{batchUC: batchExecutorStub{cmd: &captured}}
	if _, err := h.handleLocalBatch(context.Background(), application.ProcessBatchCommand{},
		afirmauri.Solicitud{Operacion: afirmauri.OperacionLote},
		afirmauri.BatchMetadata{IDs: []string{"d1"}, StopOnError: true}); err == nil {
		t.Fatal("sin selector configurado el lote local debía fallar cerrado")
	}
	if captured.StopOnError {
		t.Fatal("no debía llegar a ejecutarse el lote")
	}
}
