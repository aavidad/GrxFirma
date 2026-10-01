// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

func TestODFDetached_SignAndVerify(t *testing.T) {
	priv, cert := certForTest(t, "ODF-Sign")
	key := &LocalSigningKey{ID: "odf-key", Signer: priv, Certificate: cert}
	unsigned := makeZipBytes(t, map[string][]byte{
		"mimetype":              []byte("application/vnd.oasis.opendocument.text"),
		"META-INF/manifest.xml": []byte(`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`),
		"content.xml":           []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`),
	})

	engine := NewODFDetached()
	signed, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "prueba.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: unsigned},
		Format:   formatODF,
		Action:   domain.ActionSign,
	}, key)
	if err != nil {
		t.Fatalf("Sign(ODF) error = %v", err)
	}
	if signed.Format != formatODF {
		t.Fatalf("formato firmado inesperado: %s", signed.Format)
	}

	verification, signers, err := NewODFVerifier().Verify(context.Background(), domain.Document{
		Name:     "prueba.odt",
		MIMEType: "application/vnd.oasis.opendocument.text",
		Content:  signed.Data,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(ODF) error = %v", err)
	}
	if !verification.Valid {
		t.Fatalf("la firma ODF debería ser válida: %s", verification.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba 1 firmante ODF, obtenidos %d", len(signers))
	}
}

func TestOOXMLDetached_SignAndVerify(t *testing.T) {
	priv, cert := certForTest(t, "OOXML-Sign")
	key := &LocalSigningKey{ID: "ooxml-key", Signer: priv, Certificate: cert}
	unsigned := makeZipBytes(t, map[string][]byte{
		"[Content_Types].xml": []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`),
		"_rels/.rels":         []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`),
		"docProps/app.xml":    []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`),
		"docProps/core.xml":   []byte(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`),
		"word/document.xml":   []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`),
	})

	engine := NewOOXMLDetached()
	signed, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "prueba.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Content: unsigned},
		Format:   formatOOXML,
		Action:   domain.ActionSign,
	}, key)
	if err != nil {
		t.Fatalf("Sign(OOXML) error = %v", err)
	}
	if signed.Format != formatOOXML {
		t.Fatalf("formato firmado inesperado: %s", signed.Format)
	}

	verification, signers, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{
		Name:     "prueba.docx",
		MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Content:  signed.Data,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(OOXML) error = %v", err)
	}
	if !verification.Valid {
		t.Fatalf("la firma OOXML debería ser válida: %s", verification.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba 1 firmante OOXML, obtenidos %d", len(signers))
	}
	if !strings.Contains(string(signed.Data), "_xmlsignatures/origin.sigs") {
		t.Fatal("el OOXML firmado debe contener scaffolding _xmlsignatures")
	}

	archive, err := openMutableZip(signed.Data)
	if err != nil {
		t.Fatalf("openMutableZip(firmado) error = %v", err)
	}
	contentTypes, ok := archive.Get("[Content_Types].xml")
	if !ok {
		t.Fatal("el OOXML firmado no contiene [Content_Types].xml")
	}
	tamperedContentTypes := bytes.ReplaceAll(
		contentTypes,
		[]byte("application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"),
		[]byte("application/xml"),
	)
	if bytes.Equal(tamperedContentTypes, contentTypes) {
		t.Fatal("la fixture no permitió alterar el content type")
	}
	archive.Put("[Content_Types].xml", tamperedContentTypes)
	tampered, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(OOXML alterado) error = %v", err)
	}
	if _, _, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{
		Name:     "prueba.docx",
		MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Content:  tampered,
	}, domain.CertificateChain{}); err == nil {
		t.Fatal("Verify(OOXML) aceptó un content type distinto del firmado")
	}
}

func TestOfficeDetached_CoSignPreservesAllSigners(t *testing.T) {
	odfUnsigned := makeZipBytes(t, map[string][]byte{
		"mimetype":              []byte("application/vnd.oasis.opendocument.text"),
		"META-INF/manifest.xml": []byte(`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`),
		"content.xml":           []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`),
	})
	ooxmlUnsigned := makeZipBytes(t, map[string][]byte{
		"[Content_Types].xml": []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`),
		"_rels/.rels":         []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`),
		"docProps/app.xml":    []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`),
		"docProps/core.xml":   []byte(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`),
		"word/document.xml":   []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`),
	})

	for _, tc := range []struct {
		name     string
		format   domain.SignatureFormat
		document domain.Document
		sign     func(context.Context, domain.SignatureJob, ports.SigningKey) (domain.SignatureResult, error)
		verify   func(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
	}{
		{
			name:     "ODF",
			format:   formatODF,
			document: domain.Document{Name: "cofirma.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: odfUnsigned},
			sign:     NewODFDetached().Sign,
			verify:   NewODFVerifier().Verify,
		},
		{
			name:     "OOXML",
			format:   formatOOXML,
			document: domain.Document{Name: "cofirma.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Content: ooxmlUnsigned},
			sign:     NewOOXMLDetached().Sign,
			verify:   NewOOXMLVerifier().Verify,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			firstPrivate, firstCertificate := certForTest(t, tc.name+"-CoSign-1")
			secondPrivate, secondCertificate := certForTest(t, tc.name+"-CoSign-2")
			first, err := tc.sign(context.Background(), domain.SignatureJob{
				Document: tc.document,
				Format:   tc.format,
				Action:   domain.ActionSign,
			}, &LocalSigningKey{ID: "first", Signer: firstPrivate, Certificate: firstCertificate})
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}
			second, err := tc.sign(context.Background(), domain.SignatureJob{
				Document: domain.Document{Name: tc.document.Name, MIMEType: tc.document.MIMEType, Content: first.Data},
				Format:   tc.format,
				Action:   domain.ActionCoSign,
			}, &LocalSigningKey{ID: "second", Signer: secondPrivate, Certificate: secondCertificate})
			if err != nil {
				t.Fatalf("CoSign() error = %v", err)
			}
			verification, signers, err := tc.verify(context.Background(), domain.Document{
				Name:     tc.document.Name,
				MIMEType: tc.document.MIMEType,
				Content:  second.Data,
			}, domain.CertificateChain{})
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if !verification.Valid {
				t.Fatalf("la cofirma %s no es válida: %s", tc.name, verification.Reason)
			}
			if len(signers) != 2 {
				t.Fatalf("Verify() devolvió %d firmantes, want 2", len(signers))
			}
			if tc.format == formatOOXML {
				assertOOXMLCoSignScaffolding(t, second.Data)
			}
		})
	}
}

func assertOOXMLCoSignScaffolding(t *testing.T, signed []byte) {
	t.Helper()

	archive, err := openMutableZip(signed)
	if err != nil {
		t.Fatalf("openMutableZip(cofirma OOXML) error = %v", err)
	}
	for _, name := range []string{"_xmlsignatures/sig1.xml", "_xmlsignatures/sig2.xml"} {
		if _, ok := archive.Get(name); !ok {
			t.Errorf("la cofirma OOXML no contiene %s", name)
		}
	}

	originRelsData, ok := archive.Get("_xmlsignatures/_rels/origin.sigs.rels")
	if !ok {
		t.Fatal("la cofirma OOXML no contiene las relaciones del origen")
	}
	originRels, err := parseOOXMLRelationships(originRelsData)
	if err != nil {
		t.Fatalf("parseOOXMLRelationships(cofirma OOXML) error = %v", err)
	}
	if len(originRels.Relationships) != 2 {
		t.Fatalf("relaciones de firma OOXML = %d, want 2", len(originRels.Relationships))
	}

	ids := make(map[string]struct{}, len(originRels.Relationships))
	targets := make(map[string]struct{}, len(originRels.Relationships))
	for _, relationship := range originRels.Relationships {
		normalizedID := strings.ToLower(strings.TrimSpace(relationship.ID))
		if _, exists := ids[normalizedID]; exists {
			t.Errorf("identificador de relación OOXML duplicado: %q", relationship.ID)
		}
		ids[normalizedID] = struct{}{}
		targets[strings.ToLower(strings.TrimSpace(relationship.Target))] = struct{}{}
	}
	for _, target := range []string{"sig1.xml", "sig2.xml"} {
		if _, ok := targets[target]; !ok {
			t.Errorf("relaciones de cofirma OOXML sin destino %s", target)
		}
	}
}

func TestNextOOXMLSignatureEntry_AvoidsCaseInsensitiveCollisions(t *testing.T) {
	archive := &mutableZip{entries: map[string][]byte{
		"_xmlsignatures/SIG1.XML": nil,
		"_xmlsignatures/sig2.xml": nil,
	}}

	if got := nextOOXMLSignatureEntry(archive); got != "_xmlsignatures/sig3.xml" {
		t.Fatalf("nextOOXMLSignatureEntry() = %q, want %q", got, "_xmlsignatures/sig3.xml")
	}
}

func TestNextOOXMLRelationshipID_AvoidsCaseInsensitiveCollisions(t *testing.T) {
	items := []ooxmlRelationship{{ID: "REL-1"}, {ID: "rel-2"}}

	if got := nextOOXMLRelationshipID(items, "rel-"); got != "rel-3" {
		t.Fatalf("nextOOXMLRelationshipID() = %q, want %q", got, "rel-3")
	}
}
