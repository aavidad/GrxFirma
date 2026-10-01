// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package officecontainer

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestDetectOOXMLAndExtractSignatureXMLs(t *testing.T) {
	doc := makeZIPFixture(t, map[string]string{
		"[Content_Types].xml":                   "<Types/>",
		"_rels/.rels":                           `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`,
		"docProps/app.xml":                      "<Properties/>",
		"docProps/core.xml":                     "<coreProperties/>",
		"_xmlsignatures/origin.sigs":            "",
		"_xmlsignatures/_rels/origin.sigs.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="sig1.xml"/></Relationships>`,
		"_xmlsignatures/sig1.xml":               `<Signature xmlns="http://www.w3.org/2000/09/xmldsig#"><SignedInfo/></Signature>`,
	})

	if got := Detect(doc); got != KindOOXML {
		t.Fatalf("Detect() = %q, want %q", got, KindOOXML)
	}
	if got := SignedDetect(doc); got != KindOOXML {
		t.Fatalf("SignedDetect() = %q, want %q", got, KindOOXML)
	}

	sigs, err := ExtractOOXMLSignatureXMLs(doc)
	if err != nil {
		t.Fatalf("ExtractOOXMLSignatureXMLs() error = %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("se esperaba 1 firma OOXML, obtenidas %d", len(sigs))
	}
	if !strings.Contains(string(sigs["_xmlsignatures/sig1.xml"]), "<Signature") {
		t.Fatal("el XML extraido no parece una firma OOXML")
	}
}

func TestDetectODFAndExtractSignatureXML(t *testing.T) {
	doc := makeZIPFixture(t, map[string]string{
		"mimetype":                        "application/vnd.oasis.opendocument.text",
		"META-INF/manifest.xml":           `<manifest:file-entry xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"/>`,
		"META-INF/documentsignatures.xml": `<?xml version="1.0" encoding="UTF-8"?><document-signatures xmlns="urn:oasis:names:tc:opendocument:xmlns:digitalsignature:1.0"><Signature xmlns="http://www.w3.org/2000/09/xmldsig#"/></document-signatures>`,
	})

	if got := Detect(doc); got != KindODF {
		t.Fatalf("Detect() = %q, want %q", got, KindODF)
	}
	if got := SignedDetect(doc); got != KindODF {
		t.Fatalf("SignedDetect() = %q, want %q", got, KindODF)
	}

	sigXML, err := ExtractODFSignatureXML(doc)
	if err != nil {
		t.Fatalf("ExtractODFSignatureXML() error = %v", err)
	}
	if !strings.Contains(string(sigXML), "document-signatures") {
		t.Fatal("el XML extraido no parece el envelope ODF")
	}
}

func TestSignedDetectRejectsUnsignedContainers(t *testing.T) {
	odf := makeZIPFixture(t, map[string]string{
		"mimetype":              "application/vnd.oasis.opendocument.text",
		"META-INF/manifest.xml": `<manifest:file-entry xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"/>`,
	})
	ooxml := makeZIPFixture(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"_rels/.rels":         "<Relationships/>",
		"docProps/app.xml":    "<Properties/>",
		"docProps/core.xml":   "<coreProperties/>",
	})

	if got := SignedDetect(odf); got != KindUnknown {
		t.Fatalf("SignedDetect(odf unsigned) = %q, want %q", got, KindUnknown)
	}
	if got := SignedDetect(ooxml); got != KindUnknown {
		t.Fatalf("SignedDetect(ooxml unsigned) = %q, want %q", got, KindUnknown)
	}
}

func makeZIPFixture(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creando entrada ZIP %q: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("escribiendo entrada ZIP %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("cerrando ZIP: %v", err)
	}
	return buf.Bytes()
}
