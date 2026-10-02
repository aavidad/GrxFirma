// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

type xmlTrustFixture struct {
	root    *x509.Certificate
	leaf    *x509.Certificate
	leafKey *rsa.PrivateKey
}

func TestXMLVerifiers_EvaluanConfianzaX509(t *testing.T) {
	fixture := newXMLTrustFixture(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	otherRoot := newXMLTrustFixture(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)).root

	tests := []struct {
		name   string
		sign   func(*testing.T, xmlTrustFixture) domain.Document
		verify func(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
	}{
		{name: "XAdES", sign: signXAdESTrustDocument, verify: NewXAdESVerifier().Verify},
		{name: "XMLDSig", sign: signXMLDSigTrustDocument, verify: NewXMLDSigVerifier().Verify},
		{name: "FacturaE", sign: signFacturaETrustDocument, verify: NewFacturaEVerifier().Verify},
		{name: "ASiC-XAdES", sign: signASiCXAdESTrustDocument, verify: NewASiCXAdESVerifier().Verify},
		{name: "ODF", sign: signODFTrustDocument, verify: NewODFVerifier().Verify},
		{name: "OOXML", sign: signOOXMLTrustDocument, verify: NewOOXMLVerifier().Verify},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			document := tc.sign(t, fixture)

			withoutAnchors, _, err := tc.verify(context.Background(), document, domain.CertificateChain{})
			if err != nil {
				t.Fatalf("Verify(sin anclas) error = %v", err)
			}
			if withoutAnchors.Valid || withoutAnchors.Reason != "revocación no concluyente" || withoutAnchors.Integrity.Status != domain.VerificationStatusValid {
				t.Fatalf("integridad=%s, valid=%v, motivo=%q", withoutAnchors.Integrity.Status, withoutAnchors.Valid, withoutAnchors.Reason)
			}
			if withoutAnchors.Certificate.Status != domain.VerificationStatusUnknown && withoutAnchors.Certificate.Status != domain.VerificationStatusWarning {
				t.Fatalf("Certificate.Status=%q, want valid", withoutAnchors.Certificate.Status)
			}
			if withoutAnchors.Trust.Status != domain.VerificationStatusUnknown {
				t.Fatalf("Trust.Status=%q sin anclas, want unknown", withoutAnchors.Trust.Status)
			}

			trusted, _, err := tc.verify(context.Background(), document, domain.CertificateChain{
				DERCertificates: [][]byte{fixture.root.Raw},
			})
			if err != nil {
				t.Fatalf("Verify(raíz confiable) error = %v", err)
			}
			if trusted.Valid || trusted.Reason != "revocación no concluyente" || trusted.Trust.Status != domain.VerificationStatusValid {
				t.Fatalf("la raíz emisora debe validar la confianza: valid=%v trust=%+v", trusted.Valid, trusted.Trust)
			}

			untrusted, _, err := tc.verify(context.Background(), document, domain.CertificateChain{
				DERCertificates: [][]byte{otherRoot.Raw},
			})
			if err != nil {
				t.Fatalf("Verify(raíz ajena) error = %v", err)
			}
			if untrusted.Valid {
				t.Fatal("una raíz ajena no puede producir Valid=true")
			}
			if untrusted.Integrity.Status != domain.VerificationStatusValid {
				t.Fatalf("la raíz ajena no debe contaminar la integridad: %+v", untrusted.Integrity)
			}
			if untrusted.Trust.Status != domain.VerificationStatusInvalid {
				t.Fatalf("Trust.Status=%q con raíz ajena, want invalid", untrusted.Trust.Status)
			}
		})
	}
}

func TestXAdESVerifier_CertificadoCaducadoNoProduceValido(t *testing.T) {
	fixture := newXMLTrustFixture(t, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	document := signXAdESTrustDocument(t, fixture)

	result, _, err := NewXAdESVerifier().Verify(context.Background(), document, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result.Valid {
		t.Fatal("un certificado XAdES caducado no puede producir Valid=true")
	}
	if result.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("Integrity.Status=%q, want valid", result.Integrity.Status)
	}
	if result.Certificate.Status != domain.VerificationStatusInvalid {
		t.Fatalf("Certificate.Status=%q, want invalid", result.Certificate.Status)
	}
}

func newXMLTrustFixture(t *testing.T, leafNotBefore, leafNotAfter time.Time) xmlTrustFixture {
	t.Helper()

	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey(root) error = %v", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(5001),
		Subject:               pkix.Name{CommonName: "Raíz XML GrxFirma"},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	root := createRSACertificate(t, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey(leaf) error = %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(5002),
		Subject:               pkix.Name{CommonName: "Firmante XML GrxFirma"},
		NotBefore:             leafNotBefore,
		NotAfter:              leafNotAfter,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	leaf := createRSACertificate(t, leafTemplate, root, &leafKey.PublicKey, rootKey)
	return xmlTrustFixture{root: root, leaf: leaf, leafKey: leafKey}
}

func createRSACertificate(t *testing.T, template, parent *x509.Certificate, publicKey any, signerKey *rsa.PrivateKey) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, signerKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return certificate
}

func signXAdESTrustDocument(t *testing.T, fixture xmlTrustFixture) domain.Document {
	t.Helper()
	document := domain.Document{Name: "confianza.xml", MIMEType: "application/xml", Content: []byte("<root>confianza</root>")}
	result, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: document,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "xml-trust", Signer: fixture.leafKey, Certificate: fixture.leaf})
	if err != nil {
		t.Fatalf("Sign(XAdES) error = %v", err)
	}
	return domain.Document{Name: "confianza.xsig", MIMEType: "application/xml", Content: result.Data}
}

func signXMLDSigTrustDocument(t *testing.T, fixture xmlTrustFixture) domain.Document {
	t.Helper()
	document := domain.Document{Name: "confianza.xml", MIMEType: "application/xml", Content: []byte("<root>confianza</root>")}
	result, err := NewXMLDSigDetached().Sign(context.Background(), domain.SignatureJob{
		Document: document,
		Format:   formatXMLDSig,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "xmldsig-trust", Signer: fixture.leafKey, Certificate: fixture.leaf})
	if err != nil {
		t.Fatalf("Sign(XMLDSig) error = %v", err)
	}
	return domain.Document{Name: "confianza.dsig", MIMEType: "application/xmldsig+xml", Content: result.Data}
}

func signFacturaETrustDocument(t *testing.T, fixture xmlTrustFixture) domain.Document {
	t.Helper()
	xmlData, err := os.ReadFile("testdata/sample-facturae.xml")
	if err != nil {
		t.Fatalf("leyendo factura de ejemplo: %v", err)
	}
	result, err := NewFacturaESigner().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "factura.xml", MIMEType: "application/xml", Content: xmlData},
		Format:   formatFacturaE,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "facturae-trust", Signer: fixture.leafKey, Certificate: fixture.leaf})
	if err != nil {
		t.Fatalf("Sign(FacturaE) error = %v", err)
	}
	return domain.Document{Name: "factura_firmada.xml", MIMEType: "application/xml", Content: result.Data}
}

func signASiCXAdESTrustDocument(t *testing.T, fixture xmlTrustFixture) domain.Document {
	t.Helper()
	result, err := NewASiCXAdESSigner().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "payload.xml", MIMEType: "application/xml", Content: []byte("<root>confianza</root>")},
		Format:   formatASiCXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "asic-trust", Signer: fixture.leafKey, Certificate: fixture.leaf})
	if err != nil {
		t.Fatalf("Sign(ASiC-XAdES) error = %v", err)
	}
	return domain.Document{Name: "confianza.asics", MIMEType: "application/vnd.etsi.asic-s+zip", Content: result.Data}
}

func signODFTrustDocument(t *testing.T, fixture xmlTrustFixture) domain.Document {
	t.Helper()
	unsigned := makeZipBytes(t, map[string][]byte{
		"mimetype":              []byte("application/vnd.oasis.opendocument.text"),
		"META-INF/manifest.xml": []byte(`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`),
		"content.xml":           []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`),
	})
	result, err := NewODFDetached().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "confianza.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: unsigned},
		Format:   formatODF,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "odf-trust", Signer: fixture.leafKey, Certificate: fixture.leaf})
	if err != nil {
		t.Fatalf("Sign(ODF) error = %v", err)
	}
	return domain.Document{Name: "confianza.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: result.Data}
}

func signOOXMLTrustDocument(t *testing.T, fixture xmlTrustFixture) domain.Document {
	t.Helper()
	unsigned := makeZipBytes(t, map[string][]byte{
		"[Content_Types].xml": []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`),
		"_rels/.rels":         []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`),
		"docProps/app.xml":    []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`),
		"docProps/core.xml":   []byte(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`),
		"word/document.xml":   []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`),
	})
	result, err := NewOOXMLDetached().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "confianza.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Content: unsigned},
		Format:   formatOOXML,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "ooxml-trust", Signer: fixture.leafKey, Certificate: fixture.leaf})
	if err != nil {
		t.Fatalf("Sign(OOXML) error = %v", err)
	}
	return domain.Document{Name: "confianza.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Content: result.Data}
}
