// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"encoding/base64"
	"testing"

	"grxfirma/internal/testsupport/pdffixture"
)

// Opciones del sello que Android calcula por PDF (una página, posición relativa).
func batchSealOptions() map[string]string {
	return map[string]string{
		"visibleSeal":                   "true",
		"visibleSealPlacements":         `[{"page":1,"rect":{"x":0.6,"y":0.06,"w":0.32,"h":0.11},"rotation":0}]`,
		"visibleSealRectW":              "190",
		"visibleSealRectH":              "90",
		"visibleSealLogoOpacityPercent": "100",
		"visibleSealKeepText":           "true",
		"layer2FontColor":               "black",
	}
}

func TestBatchSignsPDFsWithVisibleSealAndExternalSigner(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	cardID, card := installSimulatedDNIe(t, facade)
	pdf := base64.StdEncoding.EncodeToString(pdffixture.Minimal())
	items := []batchItemRequest{
		{Name: "uno.pdf", MIMEType: "application/pdf", ContentBase64: pdf, Format: "pades", Options: batchSealOptions()},
		{Name: "dos.pdf", MIMEType: "application/pdf", ContentBase64: pdf, Format: "pades", Options: batchSealOptions()},
		{Name: "tres.txt", MIMEType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString([]byte("tres")), Format: "cades"},
	}
	raw, err := facade.ProcessBatchJSON(mustJSON(t, mobileBatchRequest{Items: items, CertificateID: cardID}))
	if err != nil {
		t.Fatal(err)
	}
	var response mobileBatchResponse
	decodeResponse(t, raw, &response)
	if !response.OK || len(response.Items) != 3 {
		t.Fatalf("lote con sello: %+v", response)
	}
	if card.calls != 3 {
		t.Fatalf("el DNIe debe firmar un resumen por documento: %d", card.calls)
	}
	for index := 0; index < 2; index++ {
		signed, err := base64.StdEncoding.DecodeString(response.Items[index].SignedContentBase64)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(signed, []byte("/Subtype /Widget")) && !bytes.Contains(signed, []byte("/Subtype/Widget")) {
			t.Fatalf("documento %d sin anotación visible", index)
		}
	}
}

func TestBatchCosignsSignedDocuments(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "cofirma"), "cofirma")
	first, err := facade.SignJSON(mustJSON(t, signRequest{Name: "a.xml", MIMEType: "application/xml", Format: "xades",
		CertificateID: id, ContentBase64: base64.StdEncoding.EncodeToString([]byte("<a>uno</a>"))}))
	if err != nil {
		t.Fatal(err)
	}
	var signed signResponse
	decodeResponse(t, first, &signed)
	items := []batchItemRequest{
		{Name: "a.xsig", MIMEType: "application/xml", ContentBase64: signed.SignedContentBase64, Format: "xades", Action: "cosign"},
		// Un documento sin firma no se puede cofirmar: falla solo en su posición.
		{Name: "b.xml", MIMEType: "application/xml", ContentBase64: base64.StdEncoding.EncodeToString([]byte("<b/>")), Format: "xades", Action: "cosign"},
	}
	raw, err := facade.ProcessBatchJSON(mustJSON(t, mobileBatchRequest{Items: items, CertificateID: id}))
	if err != nil {
		t.Fatal(err)
	}
	var response mobileBatchResponse
	decodeResponse(t, raw, &response)
	if response.OK || !response.Items[0].OK || response.Items[1].OK {
		t.Fatalf("cofirma en lote: %+v", response)
	}
	cosigned, err := base64.StdEncoding.DecodeString(response.Items[0].SignedContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(cosigned, []byte("SignatureValue")) < 4 {
		t.Fatal("la cofirma debe conservar la firma anterior y añadir otra")
	}
}
