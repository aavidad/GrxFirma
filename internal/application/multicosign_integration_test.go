// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type proveedorClavesPorCertificado map[string]ports.SigningKey

func (p proveedorClavesPorCertificado) KeyFor(_ context.Context, cert domain.CertificateRef) (ports.SigningKey, error) {
	key, ok := p[cert.ID]
	if !ok {
		return nil, fmt.Errorf("clave no encontrada para %s", cert.ID)
	}
	return key, nil
}

func TestMultiCoSignUseCase_PAdESRealPreservaTresFirmantes(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "test", "prueba1.pdf"))
	if err != nil {
		t.Fatalf("no se pudo leer el PDF de prueba: %v", err)
	}
	document, err := domain.NewDocument("prueba1.pdf", original, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	refs := make([]domain.CertificateRef, 0, 3)
	keys := make(proveedorClavesPorCertificado, 3)
	for idx := 1; idx <= 3; idx++ {
		id := fmt.Sprintf("cert-%d", idx)
		key, cert := generarFirmantePAdES(t, idx)
		refs = append(refs, domain.CertificateRef{
			ID:          id,
			Subject:     cert.Subject.String(),
			Issuer:      cert.Issuer.String(),
			NotAfter:    cert.NotAfter,
			Fingerprint: id,
		})
		keys[id] = desktopsigner.NuevaClaveLocal(key, cert)
	}

	signUseCase := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: refs},
		keys,
		desktopsigner.NuevoMotorFirmaGo(nil),
		&aprobadorMock{aprobado: true},
		nil,
		nil,
	)
	result, err := application.NuevoMultiCoSignUseCase(signUseCase).Execute(
		context.Background(),
		application.MultiCoSignCommand{
			Document:                 document,
			Format:                   domain.FormatPAdES,
			InitialAction:            domain.ActionSign,
			PrimaryCertificateID:     "cert-1",
			AdditionalCertificateIDs: []string{"cert-2", "cert-3"},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := bytes.Count(result.Result.Data, []byte("/ByteRange")); got != 3 {
		t.Fatalf("el PDF final contiene %d firmas, want 3", got)
	}

	signedDocument, err := domain.NewDocument("prueba1_cofirmada.pdf", result.Result.Data, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(firmado) error = %v", err)
	}
	verification, signers, err := commonsigner.NewPAdESVerifier().Verify(
		context.Background(),
		signedDocument,
		domain.CertificateChain{},
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !verification.Valid {
		t.Fatalf("el PAdES con tres firmas no es válido: %s", verification.Reason)
	}
	if len(signers) != 3 {
		t.Fatalf("Verify() devolvió %d firmantes, want 3", len(signers))
	}
	for idx, signer := range signers {
		want := fmt.Sprintf("CN=Firmante PAdES %d", idx+1)
		if signer.Subject != want {
			t.Errorf("firmante[%d] = %q, want %q", idx, signer.Subject, want)
		}
	}
}

func TestMultiCoSignUseCase_OfficeRealPreservaTresFirmantes(t *testing.T) {
	refs := make([]domain.CertificateRef, 0, 3)
	keys := make(proveedorClavesPorCertificado, 3)
	for idx := 1; idx <= 3; idx++ {
		id := fmt.Sprintf("office-cert-%d", idx)
		key, cert := generarFirmantePAdES(t, idx+10)
		refs = append(refs, domain.CertificateRef{
			ID:          id,
			Subject:     cert.Subject.String(),
			Issuer:      cert.Issuer.String(),
			NotAfter:    cert.NotAfter,
			Fingerprint: id,
		})
		keys[id] = desktopsigner.NuevaClaveLocal(key, cert)
	}

	for _, tc := range []struct {
		name     string
		format   domain.SignatureFormat
		document domain.Document
		verify   func(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
	}{
		{
			name:     "ODF",
			format:   domain.SignatureFormat("ODF"),
			document: documentoOfficePrueba(t, "prueba.odt", "application/vnd.oasis.opendocument.text", contenidoODFPrueba()),
			verify:   commonsigner.NewODFVerifier().Verify,
		},
		{
			name:     "OOXML",
			format:   domain.SignatureFormat("OOXML"),
			document: documentoOfficePrueba(t, "prueba.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", contenidoOOXMLPrueba()),
			verify:   commonsigner.NewOOXMLVerifier().Verify,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signUseCase := application.NuevoSignDocumentUseCase(
				&catalogoMock{certs: refs},
				keys,
				desktopsigner.NuevoMotorFirmaGo(nil),
				&aprobadorMock{aprobado: true},
				nil,
				nil,
			)
			result, err := application.NuevoMultiCoSignUseCase(signUseCase).Execute(
				context.Background(),
				application.MultiCoSignCommand{
					Document:                 tc.document,
					Format:                   tc.format,
					InitialAction:            domain.ActionSign,
					PrimaryCertificateID:     "office-cert-1",
					AdditionalCertificateIDs: []string{"office-cert-2", "office-cert-3"},
				},
			)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			signedDocument, err := domain.NewDocument(tc.document.Name, result.Result.Data, tc.document.MIMEType)
			if err != nil {
				t.Fatalf("NewDocument(firmado) error = %v", err)
			}
			verification, signers, err := tc.verify(context.Background(), signedDocument, domain.CertificateChain{})
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if !verification.Valid {
				t.Fatalf("%s con tres firmas no es válido: %s", tc.name, verification.Reason)
			}
			if len(signers) != 3 {
				t.Fatalf("Verify() devolvió %d firmantes, want 3", len(signers))
			}
		})
	}
}

func documentoOfficePrueba(t *testing.T, name, mimeType string, files map[string]string) domain.Document {
	t.Helper()
	var content bytes.Buffer
	writer := zip.NewWriter(&content)
	for name, value := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if name == "mimetype" {
			header.Method = zip.Store
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("zip.CreateHeader(%s) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(value)); err != nil {
			t.Fatalf("zip.Write(%s) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip.Close() error = %v", err)
	}
	document, err := domain.NewDocument(name, content.Bytes(), mimeType)
	if err != nil {
		t.Fatalf("NewDocument(%s) error = %v", name, err)
	}
	return document
}

func contenidoODFPrueba() map[string]string {
	return map[string]string{
		"mimetype":              "application/vnd.oasis.opendocument.text",
		"META-INF/manifest.xml": `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`,
		"content.xml":           `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`,
	}
}

func contenidoOOXMLPrueba() map[string]string {
	return map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"docProps/app.xml":    `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`,
		"docProps/core.xml":   `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
	}
}

func generarFirmantePAdES(t *testing.T, serial int) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(int64(serial)),
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("Firmante PAdES %d", serial),
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return key, cert
}
