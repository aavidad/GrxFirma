// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

func TestDetectSignatureFormatFollowsDesktopRules(t *testing.T) {
	facturae := readFacturaESample(t)
	cases := []struct {
		name, mime string
		content    []byte
		want       string
	}{
		{"a.pdf", "application/octet-stream", nil, "pades"},
		{"documento", "application/pdf", nil, "pades"},
		{"a.docx", "application/octet-stream", nil, "ooxml"},
		{"hoja", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil, "ooxml"},
		{"a.odt", "application/octet-stream", nil, "odf"},
		{"texto", "application/vnd.oasis.opendocument.text", nil, "odf"},
		{"a.asics", "application/octet-stream", nil, "asic-xades"},
		{"a.dsig", "application/octet-stream", nil, "xmldsig"},
		{"factura.facturae.xml", "application/xml", nil, "facturae"},
		{"recibida.xml", "text/xml", facturae, "facturae"},
		{"datos.xml", "application/xml", []byte("<datos/>"), "xades"},
		{"datos.txt", "text/plain", []byte("<datos/>"), "cades"},
	}
	for _, tc := range cases {
		if got := detectSignatureFormat(tc.name, tc.mime, tc.content); got != tc.want {
			t.Errorf("detectSignatureFormat(%q, %q) = %q; se esperaba %q", tc.name, tc.mime, got, tc.want)
		}
	}
	if _, err := resolveSignatureFormat("pkcs1", "a.txt", "text/plain", nil); err == nil {
		t.Fatal("PKCS#1 no está declarado en Android")
	}
	if got, err := resolveSignatureFormat("XAdES-ASiC-S", "a.txt", "text/plain", nil); err != nil || got != "asic-xades" {
		t.Fatalf("alias ASiC: %q %v", got, err)
	}
}

func TestNewFormatsProfilesAndActions(t *testing.T) {
	tsa := "https://tsa.example/rfc3161"
	for _, format := range []string{"xmldsig", "odf", "ooxml", "facturae", "asic-xades", "verifactu"} {
		if err := validateSigningOptions(format, "sign", map[string]string{"profile": "baseline"}); err != nil {
			t.Fatalf("%s B: %v", format, err)
		}
		if err := validateSigningOptions(format, "sign", map[string]string{"profile": "t", "tsaURL": tsa}); err == nil {
			t.Fatalf("%s no genera T", format)
		}
		if err := validateSigningOptions(format, "sign", map[string]string{"profile": "baseline", "tsaURL": tsa}); err == nil {
			t.Fatalf("%s aceptaría una TSA que ignora", format)
		}
		if err := validateSigningOptions(format, "countersign", map[string]string{"profile": "baseline"}); err == nil {
			t.Fatalf("%s no admite contrafirma", format)
		}
	}
	for _, format := range []string{"xmldsig", "odf", "ooxml"} {
		if err := validateSigningOptions(format, "cosign", nil); err != nil {
			t.Fatalf("%s cofirma: %v", format, err)
		}
	}
	for _, format := range []string{"facturae", "asic-xades", "verifactu"} {
		if err := validateSigningOptions(format, "cosign", nil); err == nil {
			t.Fatalf("%s no admite cofirma", format)
		}
	}
}

func TestMobileSignsDesktopFormatsAndVerifiesThem(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "formatos"), "formatos")
	cases := []struct {
		name, mime, format string
		content            []byte
	}{
		{"datos.xml", "application/xml", "xmldsig", []byte(`<datos><a>1</a></datos>`)},
		{"texto.odt", "application/vnd.oasis.opendocument.text", "auto", minimalODT(t)},
		{"texto.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "auto", minimalDOCX(t)},
		{"factura.xml", "application/xml", "auto", readFacturaESample(t)},
		{"nota.txt", "text/plain", "asic-xades", []byte("contenido ASiC")},
		{"registro.xml", "application/xml", "verifactu", veriFactuRecord("12345678/G33", "", time.Now().UTC().Format(time.RFC3339))},
	}
	for _, tc := range cases {
		raw, err := facade.SignJSON(mustJSON(t, signRequest{
			Name: tc.name, MIMEType: tc.mime, Format: tc.format, Action: "sign", CertificateID: id,
			ContentBase64: base64.StdEncoding.EncodeToString(tc.content),
		}))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var signed signResponse
		decodeResponse(t, raw, &signed)
		raw, err = facade.VerifyJSON(mustJSON(t, verifyRequest{Name: tc.name, MIMEType: tc.mime, ContentBase64: signed.SignedContentBase64}))
		if err != nil {
			t.Fatalf("verificar %s (%s): %v", tc.name, signed.Format, err)
		}
		var verified verifyResponse
		decodeResponse(t, raw, &verified)
		if verified.IntegrityStatus != "valid" {
			t.Fatalf("%s (%s): integridad %q %v", tc.name, signed.Format, verified.IntegrityStatus, verified.Errors)
		}
	}
}

func TestRSAOnlyFormatsGiveClosedMessageForECDSA(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importEphemeralIdentity(t, facade, "ecdsa")
	_, err := facade.SignJSON(mustJSON(t, signRequest{
		Name: "datos.xml", MIMEType: "application/xml", Format: "xmldsig", CertificateID: id,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("<a/>")),
	}))
	if err == nil || err.Error() != mobileFormatRequiresRSAMessage {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestVeriFactuSigningRejectsOtherXMLWithClosedKey(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "vf"), "vf")
	_, err := facade.SignJSON(mustJSON(t, signRequest{
		Name: "otro.xml", MIMEType: "application/xml", Format: "verifactu", CertificateID: id,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("<a/>")),
	}))
	if err == nil || err.Error() != "verifactu.root" {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestContractDeclaresNewFormatsAndDocumentServices(t *testing.T) {
	raw, err := buildMobileContract("android", true)
	if err != nil {
		t.Fatal(err)
	}
	var contract mobileContract
	decodeResponse(t, raw, &contract)
	for _, service := range []string{"verifactu_validate", "eni_document", "eni_validate", "csv_legend"} {
		if !contract.Services[service] {
			t.Fatalf("servicio %s no declarado", service)
		}
	}
	if len(contract.Signing.Formats) != len(mobileFormatOrder) {
		t.Fatalf("formatos: %v", contract.Signing.Formats)
	}
	if got := contract.Signing.KeyTypesByFormat["ODF"]; len(got) != 1 || got[0] != "RSA" {
		t.Fatalf("ODF solo admite RSA: %v", got)
	}
	if got := contract.Signing.ActionsByFormat["VeriFactu"]; len(got) != 1 || got[0] != "sign" {
		t.Fatalf("Veri*Factu solo firma: %v", got)
	}
	if contract.Services["remote_exchange"] {
		t.Fatal("el intercambio remoto debe seguir apagado")
	}
}

func readFacturaESample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "internal", "adapters", "outbound", "common", "signer", "testdata", "sample-facturae.xml"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func zipForTest(t *testing.T, entries [][2]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		w, err := writer.Create(entry[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func minimalODT(t *testing.T) []byte {
	return zipForTest(t, [][2]string{
		{"mimetype", "application/vnd.oasis.opendocument.text"},
		{"META-INF/manifest.xml", `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`},
		{"content.xml", `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`},
	})
}

func minimalDOCX(t *testing.T) []byte {
	return zipForTest(t, [][2]string{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"docProps/app.xml", `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`},
		{"docProps/core.xml", `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`},
	})
}

// veriFactuRecord reproduce el registro de prueba del motor (NIF ficticio de
// la AEAT para pruebas) con la huella recalculada.
func veriFactuRecord(number, previous, date string) []byte {
	id := `<IDFactura><IDEmisorFactura>89890001K</IDEmisorFactura><NumSerieFactura>` + number + `</NumSerieFactura><FechaExpedicionFactura>01-01-2024</FechaExpedicionFactura></IDFactura>`
	chain := `<Encadenamiento><PrimerRegistro>S</PrimerRegistro></Encadenamiento>`
	if previous != "" {
		chain = `<Encadenamiento><RegistroAnterior><IDEmisorFactura>89890001K</IDEmisorFactura><NumSerieFactura>12345678/G33</NumSerieFactura><FechaExpedicionFactura>01-01-2024</FechaExpedicionFactura><Huella>` + previous + `</Huella></RegistroAnterior></Encadenamiento>`
	}
	system := `<SistemaInformatico><NombreRazon>Prueba</NombreRazon><NIF>89890001K</NIF><NombreSistemaInformatico>Prueba</NombreSistemaInformatico><IdSistemaInformatico>01</IdSistemaInformatico><Version>1</Version><NumeroInstalacion>1</NumeroInstalacion><TipoUsoPosibleSoloVerifactu>N</TipoUsoPosibleSoloVerifactu><TipoUsoPosibleMultiOT>N</TipoUsoPosibleMultiOT><IndicadorMultiplesOT>N</IndicadorMultiplesOT></SistemaInformatico>`
	middle := `<NombreRazonEmisor>Prueba</NombreRazonEmisor><TipoFactura>F1</TipoFactura><DescripcionOperacion>Prueba</DescripcionOperacion><Desglose><DetalleDesglose><CalificacionOperacion>S1</CalificacionOperacion><BaseImponibleOimporteNoSujeto>100</BaseImponibleOimporteNoSujeto></DetalleDesglose></Desglose><CuotaTotal>12.35</CuotaTotal><ImporteTotal>123.45</ImporteTotal>`
	data := []byte(`<RegistroAlta xmlns="` + commonsigner.VeriFactuNamespace + `"><IDVersion>1.0</IDVersion>` + id + middle + chain + system + `<FechaHoraHusoGenRegistro>` + date + `</FechaHoraHusoGenRegistro><TipoHuella>01</TipoHuella><Huella>HASH</Huella></RegistroAlta>`)
	hash, _ := commonsigner.RecalcularHuellaVeriFactu(data)
	return []byte(strings.Replace(string(data), "HASH", hash, 1))
}
