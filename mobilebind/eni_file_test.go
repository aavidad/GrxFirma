// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"
)

// eniDocumentForTest crea un documento ENI real a partir de una CAdES
// implícita firmada con la identidad de la sesión.
func eniDocumentForTest(t *testing.T, facade *Facade, id, text string) string {
	t.Helper()
	raw, err := facade.SignJSON(mustJSON(t, signRequest{
		Name: "acta.txt", MIMEType: "text/plain", Format: "cades", CertificateID: id,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte(text)),
	}))
	if err != nil {
		t.Fatal(err)
	}
	var signed signResponse
	decodeResponse(t, raw, &signed)
	raw, err = facade.CreateENIDocumentJSON(mustJSON(t, eniDocumentRequest{
		SignatureBase64: signed.SignedContentBase64,
		OriginalBase64:  base64.StdEncoding.EncodeToString([]byte(text)),
		Organs:          []string{"L01180877"},
		Origin:          "administracion",
		State:           "EE01",
		DocumentType:    "TD10",
		ContentFormat:   "TXT",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var created eniDocumentResponse
	decodeResponse(t, raw, &created)
	return created.ContentBase64
}

func eniFileRequestForTest(id string, docs ...eniFileDocumentRequest) eniFileRequest {
	return eniFileRequest{
		Documents:      docs,
		CertificateID:  id,
		Organs:         []string{"L01180877"},
		Classification: "L01180877_PRO_000001",
		State:          "E02",
		OpeningDate:    "2026-10-01T09:00:00+02:00",
		Interested:     []string{"Persona interesada"},
	}
}

func TestCreateENIFileSignsIndexAndValidates(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "exp"), "exp")
	first := eniDocumentForTest(t, facade, id, "primer documento")
	second := eniDocumentForTest(t, facade, id, "segundo documento")
	raw, err := facade.CreateENIFileJSON(mustJSON(t, eniFileRequestForTest(id,
		eniFileDocumentRequest{Name: "b.xml", ContentBase64: second},
		eniFileDocumentRequest{Name: "a.xml", ContentBase64: first},
	)))
	if err != nil {
		t.Fatal(err)
	}
	var created eniFileResponse
	decodeResponse(t, raw, &created)
	if !created.OK || created.Documents != 2 || len(created.Issues) != 0 {
		t.Fatalf("expediente inesperado: %+v", created)
	}
	xmlData, err := base64.StdEncoding.DecodeString(created.ContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	text := string(xmlData)
	for _, fragment := range []string{"<ds:Signature", "<eniexpmeta:Estado>E02</eniexpmeta:Estado>",
		"<eniexpmeta:Clasificacion>L01180877_PRO_000001</eniexpmeta:Clasificacion>", "OrdenDocumentoExpediente>2<"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("falta %q en el expediente", fragment)
		}
	}
	raw, err = facade.ValidateENIJSON(mustJSON(t, eniValidateRequest{ContentBase64: created.ContentBase64}))
	if err != nil {
		t.Fatal(err)
	}
	var validation eniValidateResponse
	decodeResponse(t, raw, &validation)
	if !validation.Valid {
		t.Fatalf("el expediente generado debe validar: %+v", validation)
	}
}

func TestCreateENIFileReportsDocumentsAndLimits(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "exp"), "exp")
	good := eniDocumentForTest(t, facade, id, "documento")
	raw, err := facade.CreateENIFileJSON(mustJSON(t, eniFileRequestForTest(id,
		eniFileDocumentRequest{Name: "bueno.xml", ContentBase64: good},
		eniFileDocumentRequest{Name: "otro.xml", ContentBase64: base64.StdEncoding.EncodeToString([]byte("<otro/>"))},
	)))
	if err != nil {
		t.Fatal(err)
	}
	var report eniFileResponse
	decodeResponse(t, raw, &report)
	if report.OK || len(report.Issues) != 1 || report.Issues[0].Field != "otro.xml" ||
		!strings.HasPrefix(report.Issues[0].Key, "eni.validacion.") {
		t.Fatalf("el documento no ENI debe señalarse por su nombre: %+v", report)
	}

	raw, err = facade.CreateENIFileJSON(mustJSON(t, eniFileRequestForTest(id,
		eniFileDocumentRequest{Name: "a.xml", ContentBase64: good},
		eniFileDocumentRequest{Name: "b.xml", ContentBase64: good},
	)))
	if err != nil {
		t.Fatal(err)
	}
	report = eniFileResponse{}
	decodeResponse(t, raw, &report)
	if report.OK || len(report.Issues) != 1 || report.Issues[0].Field != "b.xml" || report.Issues[0].Key != "eni.validacion.value" {
		t.Fatalf("un documento repetido debe señalarse: %+v", report)
	}

	tooMany := make([]eniFileDocumentRequest, maxENIFileDocuments+1)
	for index := range tooMany {
		tooMany[index] = eniFileDocumentRequest{Name: "d.xml", ContentBase64: good}
	}
	cases := []struct {
		name    string
		request eniFileRequest
		key     string
	}{
		{"vacío", eniFileRequestForTest(id), "eni.validacion.limit"},
		{"demasiados", eniFileRequestForTest(id, tooMany...), "eni.validacion.limit"},
		{"órgano", func() eniFileRequest {
			r := eniFileRequestForTest(id, eniFileDocumentRequest{Name: "a.xml", ContentBase64: good})
			r.Organs = []string{"granada"}
			return r
		}(), "eni.validacion.dir3"},
		{"clasificación", func() eniFileRequest {
			r := eniFileRequestForTest(id, eniFileDocumentRequest{Name: "a.xml", ContentBase64: good})
			r.Classification = ""
			return r
		}(), "eni.validacion.classification"},
		{"estado", func() eniFileRequest {
			r := eniFileRequestForTest(id, eniFileDocumentRequest{Name: "a.xml", ContentBase64: good})
			r.State = "E09"
			return r
		}(), "eni.validacion.value"},
		{"fecha", func() eniFileRequest {
			r := eniFileRequestForTest(id, eniFileDocumentRequest{Name: "a.xml", ContentBase64: good})
			r.OpeningDate = "ayer"
			return r
		}(), "eni.validacion.date"},
		{"fecha futura", func() eniFileRequest {
			r := eniFileRequestForTest(id, eniFileDocumentRequest{Name: "a.xml", ContentBase64: good})
			r.OpeningDate = time.Now().Add(72 * time.Hour).Format(time.RFC3339)
			return r
		}(), "eni.validacion.date"},
	}
	for _, tc := range cases {
		if _, err := facade.CreateENIFileJSON(mustJSON(t, tc.request)); err == nil || err.Error() != tc.key {
			t.Errorf("%s: %v; se esperaba %s", tc.name, err, tc.key)
		}
	}
	big := strings.Repeat("A", base64.StdEncoding.EncodedLen(maxENIFileInputBytes)+4)
	if _, err := facade.CreateENIFileJSON(mustJSON(t, eniFileRequestForTest(id,
		eniFileDocumentRequest{Name: "a.xml", ContentBase64: big}))); err == nil {
		t.Fatal("un expediente demasiado grande debe rechazarse")
	}
}

func TestCreateENIFileRequiresRSAAndAcceptsExternalSigner(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	rsaID := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "exp"), "exp")
	good := eniDocumentForTest(t, facade, rsaID, "documento")

	ecdsaID := importEphemeralIdentity(t, facade, "ecdsa")
	if _, err := facade.CreateENIFileJSON(mustJSON(t, eniFileRequestForTest(ecdsaID,
		eniFileDocumentRequest{Name: "a.xml", ContentBase64: good}))); err == nil || err.Error() != mobileFormatRequiresRSAMessage {
		t.Fatalf("ECDSA: %v", err)
	}

	cardID, card := installSimulatedDNIe(t, facade)
	raw, err := facade.CreateENIFileJSON(mustJSON(t, eniFileRequestForTest(cardID,
		eniFileDocumentRequest{Name: "a.xml", ContentBase64: good})))
	if err != nil {
		t.Fatal(err)
	}
	var created eniFileResponse
	decodeResponse(t, raw, &created)
	if !created.OK || card.calls != 1 {
		t.Fatalf("el índice debe firmarse una vez con el DNIe: %+v, llamadas %d", created, card.calls)
	}
}

func TestContractDeclaresWaveFourServices(t *testing.T) {
	var contract mobileContract
	decodeResponse(t, newAndroidFacadeForTest(t).MobileContractJSON(), &contract)
	for _, service := range []string{"eni_file", "batch_visible_seal", "batch_cosign", "external_signer_batch", "external_signer_protect_sign"} {
		if !contract.Services[service] {
			t.Errorf("servicio no declarado: %s", service)
		}
	}
	if contract.Limits.ENIFileDocuments != maxENIFileDocuments {
		t.Errorf("límite de documentos del expediente: %d", contract.Limits.ENIFileDocuments)
	}
}

// installSimulatedDNIe instala como identidad de sesión un firmador externo
// RSA que solo recibe resúmenes, como el DNIe por NFC.
func installSimulatedDNIe(t *testing.T, facade *Facade) (string, *simulatedCard) {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Simulated DNIe signature"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	card := &simulatedCard{private: private}
	raw, err := facade.InstallExternalIdentityJSON(mustJSON(t, externalIdentityRequest{
		CertificateBase64: base64.StdEncoding.EncodeToString(der),
	}), card)
	if err != nil {
		t.Fatal(err)
	}
	var imported importCertificateResponse
	decodeResponse(t, raw, &imported)
	return imported.CertificateID, card
}
