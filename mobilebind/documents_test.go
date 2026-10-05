// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/base64"
	"strings"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

func TestValidateVeriFactuReportsEachRecordWithKeys(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	first := veriFactuRecord("12345678/G33", "", "2024-01-01T19:20:30+01:00")
	hash, err := commonsigner.RecalcularHuellaVeriFactu(first)
	if err != nil {
		t.Fatal(err)
	}
	second := veriFactuRecord("12345679/G34", hash, "2024-01-01T19:20:35+01:00")
	raw, err := facade.ValidateVeriFactuJSON(mustJSON(t, veriFactuValidateRequest{Files: []veriFactuFileRequest{
		{Name: "b.xml", ContentBase64: base64.StdEncoding.EncodeToString(second)},
		{Name: "a.xml", ContentBase64: base64.StdEncoding.EncodeToString(first)},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	var report veriFactuValidateResponse
	decodeResponse(t, raw, &report)
	if !report.Valid || report.Errors != 0 || len(report.Records) != 2 || report.Records[0].File != "a.xml" {
		t.Fatalf("informe inesperado: %+v", report)
	}
	if report.Records[0].CalculatedHash != hash || report.Records[1].PreviousHash != hash {
		t.Fatalf("huellas: %+v", report.Records)
	}
	if issue := report.Records[0].Issues[0]; issue.Key != "verifactu.unsigned" || issue.Level != "warning" {
		t.Fatalf("aviso de registro sin firmar: %+v", issue)
	}

	tampered := []byte(strings.Replace(string(first), "123.45", "999.99", 1))
	raw, err = facade.ValidateVeriFactuJSON(mustJSON(t, veriFactuValidateRequest{Files: []veriFactuFileRequest{
		{Name: "a.xml", ContentBase64: base64.StdEncoding.EncodeToString(tampered)},
		{Name: "a.xml", ContentBase64: base64.StdEncoding.EncodeToString(first)},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	report = veriFactuValidateResponse{}
	decodeResponse(t, raw, &report)
	if report.Valid || len(report.Records) != 2 {
		t.Fatalf("un registro alterado debe invalidar el informe: %+v", report)
	}
	found := false
	for _, record := range report.Records {
		for _, issue := range record.Issues {
			found = found || issue.Key == "verifactu.hash"
		}
	}
	if !found {
		t.Fatalf("falta la clave de huella: %+v", report.Records)
	}
	if _, err := facade.ValidateVeriFactuJSON(`{"files":[]}`); err == nil || err.Error() != "verifactu.limit" {
		t.Fatalf("selección vacía: %v", err)
	}
}

func TestCreateAndValidateENIDocument(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "eni"), "eni")
	original := []byte("acta de la sesión")
	raw, err := facade.SignJSON(mustJSON(t, signRequest{
		Name: "acta.txt", MIMEType: "text/plain", Format: "cades", CertificateID: id,
		ContentBase64: base64.StdEncoding.EncodeToString(original),
	}))
	if err != nil {
		t.Fatal(err)
	}
	var signed signResponse
	decodeResponse(t, raw, &signed)
	request := eniDocumentRequest{
		SignatureBase64: signed.SignedContentBase64,
		Organs:          []string{"L01180877"},
		Origin:          "administracion",
		State:           "EE01",
		DocumentType:    "TD10",
		CaptureDate:     "2026-10-05T10:00:00+02:00",
		ContentFormat:   "TXT",
	}
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, request)); err == nil || err.Error() != eniErrorExplicitCAdES {
		t.Fatalf("la CAdES explícita necesita el original: %v", err)
	}
	request.OriginalBase64 = base64.StdEncoding.EncodeToString(original)
	raw, err = facade.CreateENIDocumentJSON(mustJSON(t, request))
	if err != nil {
		t.Fatal(err)
	}
	var created eniDocumentResponse
	decodeResponse(t, raw, &created)
	if created.SignatureType != "TF04" || created.ContentBase64 == "" {
		t.Fatalf("documento ENI inesperado: %+v", created)
	}
	raw, err = facade.ValidateENIJSON(mustJSON(t, eniValidateRequest{ContentBase64: created.ContentBase64}))
	if err != nil {
		t.Fatal(err)
	}
	var validation eniValidateResponse
	decodeResponse(t, raw, &validation)
	if !validation.Valid || len(validation.Issues) != 0 {
		t.Fatalf("el ENI generado debe validar: %+v", validation)
	}

	bad := request
	bad.Organs = []string{"granada"}
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, bad)); err == nil || err.Error() != "eni.validacion.dir3" {
		t.Fatalf("DIR3 inválido: %v", err)
	}
	bad = request
	bad.State = "EE02"
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, bad)); err == nil || err.Error() != "eni.validacion.source" {
		t.Fatalf("copia sin origen: %v", err)
	}
	bad = request
	bad.Origin = "otro"
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, bad)); err == nil || err.Error() != eniErrorOrigin {
		t.Fatalf("origen: %v", err)
	}
	unsignedPDF := request
	unsignedPDF.SignatureBase64 = base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\n%%EOF"))
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, unsignedPDF)); err == nil || err.Error() != eniErrorUnsignedPDF {
		t.Fatalf("PDF sin firma: %v", err)
	}
	raw, err = facade.ValidateENIJSON(mustJSON(t, eniValidateRequest{ContentBase64: base64.StdEncoding.EncodeToString([]byte("<otro/>"))}))
	if err != nil {
		t.Fatal(err)
	}
	validation = eniValidateResponse{}
	decodeResponse(t, raw, &validation)
	if validation.Valid || len(validation.Issues) == 0 || !strings.HasPrefix(validation.Issues[0].Key, "eni.validacion.") {
		t.Fatalf("un XML ajeno no es ENI: %+v", validation)
	}
}

func TestENICatalogsComeFromEngine(t *testing.T) {
	var catalogs struct {
		DocumentStates []string `json:"document_states"`
		DocumentTypes  []string `json:"document_types"`
		FileStates     []string `json:"file_states"`
	}
	decodeResponse(t, newAndroidFacadeForTest(t).ENICatalogsJSON(), &catalogs)
	if len(catalogs.DocumentStates) != 5 || catalogs.DocumentTypes[len(catalogs.DocumentTypes)-1] != "TD99" || len(catalogs.FileStates) != 3 {
		t.Fatalf("catálogos: %+v", catalogs)
	}
}

func TestCSVLegendNormalizesIDNAndReportsField(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	raw, err := facade.CSVLegendJSON(mustJSON(t, csvLegendRequest{Code: "ABC 123", URL: "sede.diputación.es/cotejo?csv={csv}"}))
	if err != nil {
		t.Fatal(err)
	}
	var legend csvLegendResponse
	decodeResponse(t, raw, &legend)
	if legend.URL != "https://sede.xn--diputacin-d7a.es/cotejo?csv=ABC+123" {
		t.Fatalf("URL normalizada: %q", legend.URL)
	}
	if !strings.Contains(legend.Text, "ABC 123") || !strings.Contains(legend.Text, legend.URL) {
		t.Fatalf("texto final: %q", legend.Text)
	}
	cases := []struct {
		request csvLegendRequest
		key     string
	}{
		{csvLegendRequest{URL: "https://sede.example"}, csvErrorCodeMissing},
		{csvLegendRequest{Code: strings.Repeat("A", 129), URL: "https://sede.example"}, csvErrorCodeInvalid},
		{csvLegendRequest{Code: "A"}, csvErrorURLMissing},
		{csvLegendRequest{Code: "A", URL: "http://sede.example"}, csvErrorURLInvalid},
		{csvLegendRequest{Code: "A", URL: "https://user@sede.example"}, csvErrorURLInvalid},
		{csvLegendRequest{Code: "A", URL: "https://sede.example", Text: "a\nb"}, csvErrorTextInvalid},
		{csvLegendRequest{Code: "AB\u202eC", URL: "https://sede.example"}, csvErrorCodeInvalid},
		{csvLegendRequest{Code: "A\u200bB", URL: "https://sede.example"}, csvErrorCodeInvalid},
		{csvLegendRequest{Code: "A", URL: "https://sede.example/\u2066x\u2069"}, csvErrorURLInvalid},
		{csvLegendRequest{Code: "A", URL: "https://sede.example/x?csv=\ufeff{csv}"}, csvErrorURLInvalid},
		{csvLegendRequest{Code: "A", URL: "https://sede.example", Text: "a\u202eb"}, csvErrorTextInvalid},
		{csvLegendRequest{Code: "A", URL: "https://sede.example", Text: "a\x1bb"}, csvErrorTextInvalid},
	}
	for _, tc := range cases {
		if _, err := facade.CSVLegendJSON(mustJSON(t, tc.request)); err == nil || err.Error() != tc.key {
			t.Errorf("%+v: %v; se esperaba %s", tc.request, err, tc.key)
		}
	}
}
