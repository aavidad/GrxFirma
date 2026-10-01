// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestFacturaEReferences_DeclaranCanonicalizacionDelDigest(t *testing.T) {
	private, cert := certForTest(t, "QA FacturaE referencias declaradas")
	for name, namespace := range map[string]string{
		"3.1": facturaeNamespace31, "3.2": facturaeNamespace32,
		"3.2.1": facturaeNamespace321, "3.2.2": facturaeNamespace322,
		"3.2_legacy": facturaeNamespace32Legacy,
	} {
		t.Run(name, func(t *testing.T) {
			// XML sintético para validar la firma, no una factura fiscal/XSD.
			payload := []byte(`<Facturae xmlns="` + namespace + `" xmlns:unused="urn:qa:unused" xml:lang="es"><FileHeader xmlns=""/><Parties xmlns=""/><Invoices xmlns="">QA &amp; XML</Invoices></Facturae>`)
			result, err := NewFacturaESigner().Sign(context.Background(), domain.SignatureJob{
				Document: domain.Document{Name: "qa.xml", MIMEType: "application/xml", Content: payload},
				Format:   formatFacturaE, Action: domain.ActionSign,
			}, &LocalSigningKey{Signer: private, Certificate: cert})
			if err != nil {
				t.Fatal(err)
			}
			signedInfo, value, _, err := extractXMLSignatureCore(result.Data)
			if err != nil {
				t.Fatal(err)
			}
			var info signedInfoForVerify
			if err := xml.Unmarshal(signedInfo, &info); err != nil {
				t.Fatal(err)
			}
			if len(info.References) != 3 {
				t.Fatalf("referencias=%d, esperadas=3", len(info.References))
			}
			for _, ref := range info.References {
				var canonical []byte
				if ref.URI == "" {
					if len(ref.Transforms) != 2 || ref.Transforms[0].Algorithm != algEnveloped || ref.Transforms[1].Algorithm != algExcC14N {
						t.Errorf("documento no declara enveloped seguido de C14N exclusiva: %+v", ref.Transforms)
					}
					canonical, err = canonicalizeXML(payload, algExcC14N)
				} else {
					if ref.URI != "#Certificate1" && ref.URI != "#Signature-SignedProperties" {
						t.Fatalf("URI inesperada: %q", ref.URI)
					}
					if len(ref.Transforms) != 1 || ref.Transforms[0].Algorithm != algExcC14N {
						t.Errorf("%s no declara C14N exclusiva: %+v", ref.URI, ref.Transforms)
					}
					canonical, err = canonicalizeElementByIDInDocument(result.Data, strings.TrimPrefix(ref.URI, "#"), algExcC14N)
				}
				if err != nil {
					t.Fatal(err)
				}
				if ref.DigestMethod.Algorithm != algSHA256 {
					t.Fatalf("DigestMethod inesperado: %s", ref.DigestMethod.Algorithm)
				}
				expected, err := base64.StdEncoding.DecodeString(ref.DigestValue)
				if err != nil {
					t.Fatal(err)
				}
				actual := sha256.Sum256(canonical)
				if !bytes.Equal(expected, actual[:]) {
					t.Errorf("digest contextual no coincide: %s", ref.URI)
				}
			}
			if info.CanonicalizationMethod.Algorithm != algExcC14N || info.SignatureMethod.Algorithm != algRSASHA256 {
				t.Fatal("métodos de SignedInfo inesperados")
			}
			canonical, err := canonicalizeElementInDocument(result.Data, nsXMLDSig, "SignedInfo", algExcC14N)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(canonical)
			signature, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := rsa.VerifyPKCS1v15(&private.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type facturaeCountingSigner struct {
	crypto.Signer
	calls int
}

func (s *facturaeCountingSigner) Sign(random io.Reader, digest []byte, options crypto.SignerOpts) ([]byte, error) {
	s.calls++
	return s.Signer.Sign(random, digest, options)
}

func TestFacturaE_CanonicalizacionFallidaNoInvocaClave(t *testing.T) {
	private, cert := certForTest(t, "QA FacturaE fallo previo a firma")
	key := &facturaeCountingSigner{Signer: private}
	payload := []byte(`<Facturae xmlns="` + facturaeNamespace322 + `"><FileHeader/><Parties/><Invoices><sinDeclarar:dato>QA</sinDeclarar:dato></Invoices></Facturae>`)
	result, err := NewFacturaESigner().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "qa.xml", MIMEType: "application/xml", Content: payload},
		Format:   formatFacturaE, Action: domain.ActionSign,
	}, &LocalSigningKey{Signer: key, Certificate: cert})
	if err == nil || key.calls != 0 || len(result.Data) != 0 {
		t.Fatalf("XML no canonicalizable no debe firmarse: err=%v invocaciones=%d salida=%d", err, key.calls, len(result.Data))
	}
}
