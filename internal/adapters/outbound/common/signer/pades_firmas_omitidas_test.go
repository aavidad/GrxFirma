// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"testing"

	"grxfirma/internal/domain"
)

// pdfConDiccionarioFirmaPrevio añade al PDF, antes de firmarlo, una revisión
// con un diccionario de firma del SubFilter dado. La firma posterior cubre
// todo el documento, así que solo cuenta cómo se trata ese diccionario.
func pdfConDiccionarioFirmaPrevio(t *testing.T, subFilter string) []byte {
	t.Helper()
	base := documentoPDFRealPrueba(t).Content
	tipo := "/Sig"
	if subFilter == "ETSI.RFC3161" {
		tipo = "/DocTimeStamp"
	}
	preparado := anexarActualizacionPrueba(t, base, []objetoPDFPrueba{
		{numero: 9000, cuerpo: "<</Type " + tipo + " /Filter /Adobe.PPKLite /SubFilter /" + subFilter + " /ByteRange [0 10 20 10] /Contents <00>>>"},
	}, nil)
	doc, err := domain.NewDocument("preparado.pdf", preparado, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	return firmarPDFPrueba(t, doc)
}

func contieneTexto(items []string, buscado string) bool {
	for _, item := range items {
		if item == buscado {
			return true
		}
	}
	return false
}

// E2-08: una firma con SubFilter no admitido no puede desaparecer del
// recuento: cuenta, se avisa y el PDF queda sin poder darse por válido.
func TestPAdESVerifier_CuentaFirmaConSubFilterNoAdmitido(t *testing.T) {
	vr := verificarPDF(t, pdfConDiccionarioFirmaPrevio(t, "adbe.x509.rsa_sha1"))
	if vr.Valid {
		t.Fatalf("una firma sin verificar no puede dejar el PDF como válido: %s", vr.Reason)
	}
	if !contieneTexto(vr.Details, "firmas_pdf=2") || len(vr.Warnings) == 0 {
		t.Fatalf("se esperaban firmas_pdf=2 y un aviso: %v %v", vr.Details, vr.Warnings)
	}
}

// E2-08: un sello de tiempo de documento aparece como no evaluado.
func TestPAdESVerifier_AvisaSelloDeDocumentoNoEvaluado(t *testing.T) {
	vr := verificarPDF(t, pdfConDiccionarioFirmaPrevio(t, "ETSI.RFC3161"))
	if !contieneTexto(vr.Details, "firmas_pdf=2") || len(vr.Warnings) == 0 {
		t.Fatalf("se esperaban firmas_pdf=2 y un aviso: %v %v", vr.Details, vr.Warnings)
	}
	if !vr.Valid {
		t.Fatalf("un sello de documento sin evaluar no invalida la firma que cubre todo: %s", vr.Reason)
	}
}
