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
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestODFVerifier_VerificaContenedorFirmado(t *testing.T) {
	priv, cert := certForTest(t, "ODF-Verify")
	mimetype := []byte("application/vnd.oasis.opendocument.text")
	manifest := []byte(`<manifest:file-entry xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>`)
	content := []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`)

	signatureXML := buildContainerSignatureXML(t, priv, cert, containerSignatureSpec{
		rootPrefix: "document-signatures",
		rootAttrs:  ` xmlns="urn:oasis:names:tc:opendocument:xmlns:digitalsignature:1.0"`,
		references: []containerReference{
			{URI: "mimetype", Data: mimetype},
			{URI: "META-INF/manifest.xml", Data: manifest},
			{URI: "content.xml", Data: content},
		},
	})

	container := makeZipBytes(t, map[string][]byte{
		"mimetype":                        mimetype,
		"META-INF/manifest.xml":           manifest,
		"content.xml":                     content,
		"META-INF/documentsignatures.xml": signatureXML,
	})

	verifier := NewODFVerifier()
	result, signers, err := verifier.Verify(context.Background(), domain.Document{
		Name:     "prueba.odt",
		MIMEType: "application/vnd.oasis.opendocument.text",
		Content:  container,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(ODF) error = %v", err)
	}
	if !result.Valid {
		t.Fatalf("la firma ODF debería ser válida: %s", result.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba 1 firmante ODF, obtenidos %d", len(signers))
	}
}

func TestOOXMLVerifier_VerificaContenedorFirmado(t *testing.T) {
	priv, cert := certForTest(t, "OOXML-Verify")
	contentTypes := []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`)
	rels := []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)
	app := []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`)
	core := []byte(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`)
	document := []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`)

	signatureXML := buildContainerSignatureXML(t, priv, cert, containerSignatureSpec{
		rootPrefix: "Signature",
		rootAttrs:  ` xmlns="http://www.w3.org/2000/09/xmldsig#" Id="idPackageSignature"`,
		references: []containerReference{
			{URI: "/word/document.xml?ContentType=application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", Data: document},
			{URI: "/_rels/.rels?ContentType=application/vnd.openxmlformats-package.relationships+xml", Data: rels},
		},
	})

	container := makeZipBytes(t, map[string][]byte{
		"[Content_Types].xml":                   contentTypes,
		"_rels/.rels":                           rels,
		"docProps/app.xml":                      app,
		"docProps/core.xml":                     core,
		"word/document.xml":                     document,
		"_xmlsignatures/origin.sigs":            []byte(""),
		"_xmlsignatures/_rels/origin.sigs.rels": []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/signature" Target="sig1.xml"/></Relationships>`),
		"_xmlsignatures/sig1.xml":               signatureXML,
	})

	verifier := NewOOXMLVerifier()
	result, signers, err := verifier.Verify(context.Background(), domain.Document{
		Name:     "prueba.docx",
		MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Content:  container,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(OOXML) error = %v", err)
	}
	if !result.Valid {
		t.Fatalf("la firma OOXML debería ser válida: %s", result.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba 1 firmante OOXML, obtenidos %d", len(signers))
	}
}

func TestMultiVerifier_DetectaODFyOOXMLFirmados(t *testing.T) {
	priv, cert := certForTest(t, "Office-Multi")
	sigXML := buildContainerSignatureXML(t, priv, cert, containerSignatureSpec{
		rootPrefix: "document-signatures",
		rootAttrs:  ` xmlns="urn:oasis:names:tc:opendocument:xmlns:digitalsignature:1.0"`,
		references: []containerReference{{URI: "mimetype", Data: []byte("application/vnd.oasis.opendocument.text")}},
	})
	odf := makeZipBytes(t, map[string][]byte{
		"mimetype":                        []byte("application/vnd.oasis.opendocument.text"),
		"META-INF/manifest.xml":           []byte(`<manifest:file-entry xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"/>`),
		"META-INF/documentsignatures.xml": sigXML,
	})

	multi := NewMultiVerifier()
	result, _, err := multi.Verify(context.Background(), domain.Document{
		Name:     "multi.odt",
		MIMEType: "application/vnd.oasis.opendocument.text",
		Content:  odf,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(Multi ODF) error = %v", err)
	}
	if !containsOfficeDetail(result.Details, "formato_detectado=ODF") {
		t.Fatalf("se esperaba detalle formato_detectado=ODF, detalles=%v", result.Details)
	}
}

type containerReference struct {
	URI  string
	Data []byte
}

type containerSignatureSpec struct {
	rootPrefix string
	rootAttrs  string
	references []containerReference
}

func buildContainerSignatureXML(t *testing.T, priv crypto.Signer, cert *x509.Certificate, spec containerSignatureSpec) []byte {
	t.Helper()
	certB64 := base64.StdEncoding.EncodeToString(cert.Raw)
	keyInfoID := "KeyInfo-1"
	keyInfoXML := fmt.Sprintf(`<ds:KeyInfo xmlns:ds="%s" Id="%s"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`, nsXMLDSig, keyInfoID, certB64)
	// Este fixture positivo debe declarar la receta de su KeyInfo. Las
	// variantes históricas sin Transform se prueban por separado como warning.
	keyInfoCanonical, err := exclusiveC14N(keyInfoXML)
	if err != nil {
		t.Fatal(err)
	}
	keyInfoDigest := sha256.Sum256(keyInfoCanonical)

	var refs strings.Builder
	for i, ref := range spec.references {
		digest := sha256.Sum256(ref.Data)
		refs.WriteString(fmt.Sprintf(
			`<ds:Reference Id="Reference-%d" URI="%s"><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></ds:Reference>`,
			i+1,
			ref.URI,
			algSHA256,
			base64.StdEncoding.EncodeToString(digest[:]),
		))
	}
	refs.WriteString(fmt.Sprintf(
		`<ds:Reference URI="#%s"><ds:Transforms><ds:Transform Algorithm="%s"/></ds:Transforms><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></ds:Reference>`,
		keyInfoID,
		algExcC14N,
		algSHA256,
		base64.StdEncoding.EncodeToString(keyInfoDigest[:]),
	))

	signedInfoXML := fmt.Sprintf(
		`<ds:SignedInfo xmlns:ds="%s"><ds:CanonicalizationMethod Algorithm="%s"/><ds:SignatureMethod Algorithm="%s"/>%s</ds:SignedInfo>`,
		nsXMLDSig,
		algExcC14N,
		algRSASHA256,
		refs.String(),
	)
	signedInfoC14N, err := canonicalizeSignedInfo([]byte(signedInfoXML))
	if err != nil {
		t.Fatalf("canonicalizeSignedInfo() error = %v", err)
	}
	hash := sha256.Sum256(signedInfoC14N)
	sig, err := priv.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	body := fmt.Sprintf(
		`<ds:SignatureValue>%s</ds:SignatureValue>%s`,
		base64.StdEncoding.EncodeToString(sig),
		keyInfoXML,
	)
	if spec.rootPrefix == "Signature" {
		// La firma OOXML es la raíz; un wrapper XMLDSig Signature sin
		// SignedInfo sería una segunda firma estructuralmente incompleta.
		return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><ds:Signature xmlns:ds="%s" Id="Signature-1">%s%s</ds:Signature>`, nsXMLDSig, signedInfoXML, body))
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><%s%s><ds:Signature xmlns:ds="%s" Id="Signature-1">%s%s</ds:Signature></%s>`,
		spec.rootPrefix,
		spec.rootAttrs,
		nsXMLDSig,
		signedInfoXML,
		body,
		spec.rootPrefix,
	))
}

func makeZipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func containsOfficeDetail(details []string, expected string) bool {
	for _, detail := range details {
		if detail == expected {
			return true
		}
	}
	return false
}
