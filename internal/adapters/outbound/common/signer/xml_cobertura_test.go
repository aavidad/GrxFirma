// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// Pruebas de regresión de la cobertura de las firmas XML (E1-01, E1-02 y
// E1-03 de la revisión de seguridad del 13/11/2026): cada prueba construye el
// documento atacante de verdad y exige que no salga «válido».

type entradaZIP struct {
	nombre string
	datos  []byte
}

// zipOrdenado escribe las entradas en el orden dado y admite nombres
// repetidos, como haría un atacante.
func zipOrdenado(t *testing.T, entradas []entradaZIP) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entrada := range entradas {
		w, err := zw.Create(entrada.nombre)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entrada.datos); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func leerZIP(t *testing.T, data []byte) []entradaZIP {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out []entradaZIP
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		datos, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, entradaZIP{nombre: f.Name, datos: datos})
	}
	return out
}

func noValido(t *testing.T, vr domain.VerificationResult, err error, que string) {
	t.Helper()
	if err == nil && vr.Valid {
		t.Fatalf("%s: se ha dado por válida (coverage=%q, detalles=%v)", que, vr.Coverage, vr.Details)
	}
	if err == nil && vr.Coverage == domain.CoberturaCompleta {
		t.Fatalf("%s: no puede declarar cobertura completa: %+v", que, vr.Integrity)
	}
}

func elementoLiteral(t *testing.T, documento, apertura, cierre string) string {
	t.Helper()
	inicio := strings.Index(documento, apertura)
	if inicio < 0 {
		t.Fatalf("no se encuentra %s", apertura)
	}
	fin := strings.Index(documento[inicio:], cierre)
	if fin < 0 {
		t.Fatalf("no se encuentra %s", cierre)
	}
	return documento[inicio : inicio+fin+len(cierre)]
}

// ---------------------------------------------------------------------------
// E1-02: firmas XML que solo cubren una parte del documento
// ---------------------------------------------------------------------------

// Una firma XAdES Detached legítima de la víctima (CONTENT + ds:Signature) se
// mete en una FacturaE con importes inventados. Antes salía «firma FacturaE
// valida» con la víctima como firmante.
func TestFacturaEVerifier_RechazaFirmaEnvueltaNoEnveloped(t *testing.T) {
	firmado := string(operarXAdES(t, []byte("%PDF-1.7 contrato de la víctima"), "application/pdf", domain.ActionSign, map[string]string{"format": "XAdES Detached"}, "Victima"))
	contenido := elementoLiteral(t, firmado, "<CONTENT", "</CONTENT>")
	firma := elementoLiteral(t, firmado, "<ds:Signature", "</ds:Signature>")

	factura, err := os.ReadFile("testdata/sample-facturae.xml")
	if err != nil {
		t.Fatal(err)
	}
	falsa := strings.Replace(string(factura), "127.01", "999999.99", -1)
	// Sin xmlns:xsi ni xmlns:xsd en la raíz, la C14N inclusiva del SignedInfo
	// trasplantado da los mismos octetos que en su documento original.
	raiz := elementoLiteral(t, falsa, "<Facturae", ">")
	falsa = strings.Replace(falsa, raiz, `<Facturae xmlns="http://www.facturae.es/Facturae/2007/v3.1/Facturae">`, 1)
	cierre := strings.LastIndex(falsa, "</Facturae>")
	atacante := falsa[:cierre] + `<Extensiones xmlns="">` + contenido + firma + `</Extensiones>` + falsa[cierre:]

	doc := domain.Document{Name: "factura.xsig", MIMEType: "application/xml", Content: []byte(atacante)}
	vr, _, err := NewFacturaEVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	noValido(t, vr, err, "FacturaE con firma ajena envuelta")

	vr, firmantes, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la firma de la víctima sigue siendo correcta sobre su CONTENT: %v", err)
	}
	if vr.Valid || vr.Coverage != domain.CoberturaParcial || len(firmantes) != 1 ||
		!strings.Contains(strings.Join(vr.Details, ";"), "FileHeader") {
		t.Fatalf("XAdES genérico: se esperaba cobertura parcial nombrando lo no firmado: valid=%v coverage=%q %v", vr.Valid, vr.Coverage, vr.Details)
	}
}

// Firma enveloping metida en una raíz ajena: los resúmenes cuadran, pero la
// raíz y sus datos no están firmados.
func TestXAdESVerifier_ReferenciaParcialNoEsCoberturaCompleta(t *testing.T) {
	firmado := string(operarXAdES(t, []byte("%PDF-1.7 datos"), "application/pdf", domain.ActionSign, map[string]string{"format": "XAdES Enveloping"}, "Victima"))
	firma := elementoLiteral(t, firmado, "<ds:Signature", "</ds:Signature>")
	atacante := `<pedido><importe>9999</importe>` + firma + `</pedido>`

	doc := domain.Document{Name: "pedido.xml", MIMEType: "application/xml", Content: []byte(atacante)}
	vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatal(err)
	}
	if vr.Valid || vr.Coverage != domain.CoberturaParcial || vr.Integrity.Status != domain.VerificationStatusWarning {
		t.Fatalf("se esperaba cobertura parcial sin validez: valid=%v coverage=%q integridad=%+v", vr.Valid, vr.Coverage, vr.Integrity)
	}
	// La misma firma, tal cual se generó, sí cubre todo su documento.
	original := domain.Document{Name: "f.xsig", MIMEType: "application/xml", Content: []byte(firmado)}
	if vr, _, err := NewXAdESVerifier().Verify(context.Background(), original, domain.CertificateChain{}); err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta {
		t.Fatalf("la firma enveloping original debe seguir siendo válida: err=%v valid=%v coverage=%q", err, vr.Valid, vr.Coverage)
	}
}

// Las firmas legítimas de GrxFirma siguen cubriendo todo su documento.
func TestXAdESVerifier_CoberturaCompletaEnVariantesPropias(t *testing.T) {
	for _, c := range []struct{ formato, mime, datos string }{
		{"XAdES Enveloped", "application/xml", `<factura><total>10</total></factura>`},
		{"XAdES Enveloping", "text/xml", `<doc><a>1</a></doc>`},
		{"XAdES Detached", "text/plain", "texto"},
	} {
		firmado := operarXAdES(t, []byte(c.datos), c.mime, domain.ActionSign, map[string]string{"format": c.formato}, "A")
		doc := domain.Document{Name: "f.xsig", MIMEType: "application/xml", Content: firmado}
		vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
		if err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta {
			t.Fatalf("%s: err=%v valid=%v coverage=%q %v", c.formato, err, vr.Valid, vr.Coverage, vr.Details)
		}
	}
}

// ---------------------------------------------------------------------------
// E1-01: OOXML con las partes dentro de un ds:Manifest (estructura de Office)
// ---------------------------------------------------------------------------

const (
	ctDocumentoWord = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	ctRelaciones    = "application/vnd.openxmlformats-package.relationships+xml"
	ctCore          = "application/vnd.openxmlformats-package.core-properties+xml"
	ctApp           = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
)

type parteOOXML struct {
	nombre, contentType string
	datos               []byte
	relationship        bool // transformación de relaciones de OPC, como Office
	// fuentes son los SourceId de la transformación (rId1 si no se dan) y
	// paramsRelaciones, si no está vacío, sustituye a los hijos del Transform.
	fuentes          []string
	paramsRelaciones string
	// sinFirmar deja la parte en el paquete pero fuera del Manifest, como
	// hace Office con docProps.
	sinFirmar bool
}

func partesDocx() []parteOOXML {
	return []parteOOXML{
		{nombre: "_rels/.rels", contentType: ctRelaciones, datos: []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="word/document.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"></Relationship></Relationships>`)},
		{nombre: "docProps/app.xml", contentType: ctApp, datos: []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"></Properties>`)},
		{nombre: "docProps/core.xml", contentType: ctCore, datos: []byte(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"></cp:coreProperties>`)},
		{nombre: "word/document.xml", contentType: ctDocumentoWord, datos: []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p>Pagaré 100 euros</w:p></w:body></w:document>`)},
	}
}

// firmaOfficeManifest construye una firma con la estructura de ECMA-376
// parte 2, §13: SignedInfo solo referencia #idPackageObject y las partes del
// paquete van como ds:Reference dentro de <Object Id="idPackageObject">
// <Manifest>. Las partes de la prueba ya están en forma canónica, así que su
// resumen C14N es el de sus octetos.
func firmaOfficeManifest(t *testing.T, key crypto.Signer, cert *x509.Certificate, partes []parteOOXML) []byte {
	t.Helper()
	var manifest strings.Builder
	for _, parte := range partes {
		if parte.sinFirmar {
			continue
		}
		suma := sha256.Sum256(parte.datos)
		transforms := `<Transform Algorithm="` + algC14N + `"/>`
		if parte.relationship {
			fuentes := parte.fuentes
			if len(fuentes) == 0 {
				fuentes = []string{"rId1"}
			}
			params := parte.paramsRelaciones
			sel := seleccionRelaciones{ids: map[string]bool{}, tipos: map[string]bool{}}
			for _, fuente := range fuentes {
				sel.ids[fuente] = true
				if parte.paramsRelaciones == "" {
					params += `<mdssi:RelationshipReference xmlns:mdssi="http://schemas.openxmlformats.org/package/2006/digital-signature" SourceId="` + fuente + `"/>`
				}
			}
			transforms = `<Transform Algorithm="` + algRelationshipTransform + `">` + params + `</Transform>` + transforms
			if transformado, _, err := transformarRelaciones(parte.datos, sel, algC14N); err == nil {
				suma = sha256.Sum256(transformado)
			}
		}
		fmt.Fprintf(&manifest, `<Reference URI="/%s?ContentType=%s"><Transforms>%s</Transforms><DigestMethod Algorithm="%s"/><DigestValue>%s</DigestValue></Reference>`,
			parte.nombre, parte.contentType, transforms, algSHA256, base64.StdEncoding.EncodeToString(suma[:]))
	}
	objeto := `<Object Id="idPackageObject"><Manifest>` + manifest.String() + `</Manifest>` +
		`<SignatureProperties><SignatureProperty Id="idSignatureTime" Target="#idPackageSignature"><mdssi:SignatureTime xmlns:mdssi="http://schemas.openxmlformats.org/package/2006/digital-signature"><mdssi:Format>YYYY-MM-DDThh:mm:ssTZD</mdssi:Format><mdssi:Value>2026-11-13T10:00:00Z</mdssi:Value></mdssi:SignatureTime></SignatureProperty></SignatureProperties></Object>`
	keyInfo := `<KeyInfo><X509Data><X509Certificate>` + base64.StdEncoding.EncodeToString(cert.Raw) + `</X509Certificate></X509Data></KeyInfo>`
	plantilla := func(resumenObjeto, valor string) []byte {
		return []byte(`<Signature xmlns="` + nsXMLDSig + `" Id="idPackageSignature"><SignedInfo><CanonicalizationMethod Algorithm="` + algC14N + `"/><SignatureMethod Algorithm="` + algRSASHA256 + `"/>` +
			`<Reference Type="http://www.w3.org/2000/09/xmldsig#Object" URI="#idPackageObject"><DigestMethod Algorithm="` + algSHA256 + `"/><DigestValue>` + resumenObjeto + `</DigestValue></Reference>` +
			`</SignedInfo><SignatureValue>` + valor + `</SignatureValue>` + keyInfo + objeto + `</Signature>`)
	}
	canonicoObjeto, err := canonicalizeElementByIDInDocument(plantilla("", ""), "idPackageObject", algC14N)
	if err != nil {
		t.Fatal(err)
	}
	sumaObjeto := sha256.Sum256(canonicoObjeto)
	resumen := base64.StdEncoding.EncodeToString(sumaObjeto[:])
	canonicoInfo, err := canonicalizeElementInDocument(plantilla(resumen, ""), nsXMLDSig, "SignedInfo", algC14N)
	if err != nil {
		t.Fatal(err)
	}
	sumaInfo := sha256.Sum256(canonicoInfo)
	valor, err := key.Sign(rand.Reader, sumaInfo[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return plantilla(resumen, base64.StdEncoding.EncodeToString(valor))
}

func docxConFirma(t *testing.T, partes []parteOOXML, firma []byte, cambios map[string][]byte) []byte {
	t.Helper()
	tipos := `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="` + ctRelaciones + `"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="sigs" ContentType="application/vnd.openxmlformats-package.digital-signature-origin"/>` +
		`<Override PartName="/word/document.xml" ContentType="` + ctDocumentoWord + `"/><Override PartName="/docProps/core.xml" ContentType="` + ctCore + `"/><Override PartName="/docProps/app.xml" ContentType="` + ctApp + `"/>` +
		`<Override PartName="/_xmlsignatures/sig1.xml" ContentType="application/vnd.openxmlformats-package.digital-signature-xmlsignature+xml"/></Types>`
	entradas := []entradaZIP{{nombre: "[Content_Types].xml", datos: []byte(tipos)}}
	for _, parte := range partes {
		datos := parte.datos
		if cambiado, ok := cambios[parte.nombre]; ok {
			datos = cambiado
		}
		entradas = append(entradas, entradaZIP{nombre: parte.nombre, datos: datos})
	}
	entradas = append(entradas,
		entradaZIP{nombre: "_xmlsignatures/origin.sigs"},
		entradaZIP{nombre: "_xmlsignatures/_rels/origin.sigs.rels", datos: []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/signature" Target="sig1.xml"/></Relationships>`)},
		entradaZIP{nombre: "_xmlsignatures/sig1.xml", datos: firma},
	)
	return zipOrdenado(t, entradas)
}

func TestOOXMLVerifier_ManifestDeOffice(t *testing.T) {
	key, cert := certForTest(t, "Office Manifest")
	partes := partesDocx()
	firma := firmaOfficeManifest(t, key, cert, partes)
	verificar := func(contenido []byte) (domain.VerificationResult, error) {
		vr, _, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{Name: "f.docx", Content: contenido}, domain.CertificateChain{})
		return vr, err
	}

	// A: el documento tal cual se firmó, con todas las partes comprobadas.
	vr, err := verificar(docxConFirma(t, partes, firma, nil))
	if err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta {
		t.Fatalf("Manifest original: err=%v valid=%v coverage=%q %v", err, vr.Valid, vr.Coverage, vr.Details)
	}

	// B: un byte cambiado en word/document.xml. Antes salía «válida».
	alterado := bytes.Replace(partes[3].datos, []byte("100"), []byte("900"), 1)
	vr, err = verificar(docxConFirma(t, partes, firma, map[string][]byte{"word/document.xml": alterado}))
	noValido(t, vr, err, "docx alterado tras firmar con Office")
	if err == nil {
		t.Fatalf("un resumen del Manifest que no cuadra debe invalidar la firma: %+v", vr.Integrity)
	}

	// C: una parte del Manifest que falta del paquete.
	sinParte := zipSinEntrada(t, docxConFirma(t, partes, firma, nil), "docProps/app.xml")
	vr, err = verificar(sinParte)
	noValido(t, vr, err, "docx sin una parte firmada")
}

// Office firma las partes .rels con la transformación de relaciones de OPC.
// Si la transformación trae hijos que no se saben aplicar, la parte .rels
// queda «no comprobada» y la firma nunca sale válida.
func TestOOXMLVerifier_ManifestConRelationshipTransformDesconocidaNoEsValido(t *testing.T) {
	key, cert := certForTest(t, "Office Relationship")
	partes := partesDocx()
	partes[0].relationship = true
	partes[0].paramsRelaciones = `<mdssi:OtraCosa xmlns:mdssi="http://schemas.openxmlformats.org/package/2006/digital-signature" SourceId="rId1"/>`
	firma := firmaOfficeManifest(t, key, cert, partes)
	vr, firmantes, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{Name: "f.docx", Content: docxConFirma(t, partes, firma, nil)}, domain.CertificateChain{})
	if err != nil {
		t.Fatal(err)
	}
	if vr.Valid || vr.Coverage != domain.CoberturaNoAcreditada || vr.Integrity.Status != domain.VerificationStatusUnknown || len(firmantes) != 1 ||
		!strings.Contains(strings.Join(vr.Details, ";"), "manifest_relationship_transform") {
		t.Fatalf("se esperaba «no comprobado» con motivo: valid=%v coverage=%q integridad=%+v %v", vr.Valid, vr.Coverage, vr.Integrity, vr.Details)
	}
	if vr.Reason != motivoCoberturaNoComprobada {
		t.Fatalf("motivo = %q", vr.Reason)
	}
}

func zipSinEntrada(t *testing.T, data []byte, nombre string) []byte {
	t.Helper()
	var quedan []entradaZIP
	for _, entrada := range leerZIP(t, data) {
		if entrada.nombre != nombre {
			quedan = append(quedan, entrada)
		}
	}
	return zipOrdenado(t, quedan)
}

// ---------------------------------------------------------------------------
// E1-03: contenedores ZIP con entradas repetidas, ambiguas o sin firmar
// ---------------------------------------------------------------------------

func odtFirmado(t *testing.T) []byte {
	t.Helper()
	priv, cert := certForTest(t, "ODF Victima")
	unsigned := zipOrdenado(t, []entradaZIP{
		{nombre: "mimetype", datos: []byte("application/vnd.oasis.opendocument.text")},
		{nombre: "META-INF/manifest.xml", datos: []byte(`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`)},
		{nombre: "content.xml", datos: []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body>Debo 100 euros</office:body></office:document-content>`)},
	})
	signed, err := NewODFDetached().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "v.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: unsigned},
		Format:   formatODF,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "odf", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return signed.Data
}

func verificarODF(t *testing.T, contenido []byte) (domain.VerificationResult, error) {
	t.Helper()
	vr, _, err := NewODFVerifier().Verify(context.Background(), domain.Document{Name: "v.odt", Content: contenido}, domain.CertificateChain{})
	return vr, err
}

func TestODFVerifier_RechazaEntradasEquivalentes(t *testing.T) {
	original := leerZIP(t, odtFirmado(t))
	if vr, err := verificarODF(t, zipOrdenado(t, original)); err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta {
		t.Fatalf("el ODT firmado debe ser válido: err=%v valid=%v %v", err, vr.Valid, vr.Details)
	}
	falso := entradaZIP{nombre: "content.xml", datos: []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body>Debo 900 euros</office:body></office:document-content>`)}
	renombrar := func(nuevo string) []entradaZIP {
		out := []entradaZIP{falso}
		for _, entrada := range original {
			if entrada.nombre == "content.xml" {
				entrada.nombre = nuevo
			}
			out = append(out, entrada)
		}
		return out
	}
	for _, caso := range []struct {
		nombre   string
		entradas []entradaZIP
	}{
		// LibreOffice abre la primera content.xml (la falsa); el verificador
		// leía la legítima renombrada o la última repetida.
		{"mayusculas", renombrar("Content.xml")},
		{"duplicado_exacto", renombrar("content.xml")},
		{"barra_inicial", renombrar("/content.xml")},
		{"escapado", renombrar("content%2Exml")},
		{"barra_invertida", renombrar(`.\content.xml`)},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			vr, err := verificarODF(t, zipOrdenado(t, caso.entradas))
			noValido(t, vr, err, caso.nombre)
		})
	}
}

func TestODFVerifier_AvisaEntradaNoFirmada(t *testing.T) {
	entradas := append(leerZIP(t, odtFirmado(t)), entradaZIP{nombre: "Basic/x.xml", datos: []byte(`<macro>Shell("calc")</macro>`)})
	vr, err := verificarODF(t, zipOrdenado(t, entradas))
	if err != nil {
		t.Fatal(err)
	}
	if vr.Valid || vr.Coverage != domain.CoberturaParcial || !strings.Contains(strings.Join(vr.Details, ";"), "Basic/x.xml") {
		t.Fatalf("se esperaba cobertura parcial con la entrada nombrada: valid=%v coverage=%q %v", vr.Valid, vr.Coverage, vr.Details)
	}
	if vr.Reason != motivoCoberturaParcial {
		t.Fatalf("motivo = %q", vr.Reason)
	}
}

func asicFirmado(t *testing.T) []byte {
	t.Helper()
	priv, cert := certForTest(t, "ASiC Victima")
	doc, err := domain.NewDocument("datos.txt", []byte("Debo 100 euros"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewASiCXAdESSigner().Sign(context.Background(), domain.SignatureJob{Document: doc, Format: formatASiCXAdES, Action: domain.ActionSign}, &LocalSigningKey{ID: "a", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return res.Data
}

func TestASiCXAdESVerifier_RechazaDatosAmbiguos(t *testing.T) {
	original := leerZIP(t, asicFirmado(t))
	verificar := func(entradas []entradaZIP) (domain.VerificationResult, error) {
		vr, _, err := NewASiCXAdESVerifier().Verify(context.Background(), domain.Document{Name: "d.asics", Content: zipOrdenado(t, entradas)}, domain.CertificateChain{})
		return vr, err
	}
	if vr, err := verificar(original); err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta {
		t.Fatalf("el ASiC firmado debe ser válido: err=%v valid=%v %v", err, vr.Valid, vr.Details)
	}
	falso := entradaZIP{nombre: "DATOS.txt", datos: []byte("Debo 900 euros")}
	var conFalsoDelante []entradaZIP
	for _, entrada := range original {
		if entrada.nombre == "datos.txt" {
			conFalsoDelante = append(conFalsoDelante, falso)
		}
		conFalsoDelante = append(conFalsoDelante, entrada)
	}
	vr, err := verificar(conFalsoDelante)
	noValido(t, vr, err, "ASiC con otra entrada que solo cambia mayúsculas")

	segundoDato := append(append([]entradaZIP(nil), original...), entradaZIP{nombre: "anexo.txt", datos: []byte("sin firmar")})
	vr, err = verificar(segundoDato)
	noValido(t, vr, err, "ASiC-S con dos objetos de datos")

	var renombrado []entradaZIP
	for _, entrada := range original {
		if entrada.nombre == "datos.txt" {
			entrada.nombre = "Datos.txt"
		}
		renombrado = append(renombrado, entrada)
	}
	vr, err = verificar(renombrado)
	noValido(t, vr, err, "ASiC con el dato renombrado en mayúsculas")
}
