// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// La verificación de ida y vuelta tolera variantes históricas de C14N. Esta
// regresión exige que las referencias internas nuevas declaren el algoritmo
// que realmente produjo su digest, sin probar candidatos de compatibilidad.
func TestXAdESReferences_DeclaranCanonicalizacionDelDigest(t *testing.T) {
	private, cert := certForTest(t, "QA referencias XAdES")
	key := &LocalSigningKey{Signer: private, Certificate: cert}
	payload := []byte(`<raiz xmlns="urn:qa:payload"><dato>prueba</dato></raiz>`)
	for _, tc := range []struct {
		name    string
		options map[string]string
		asic    bool
		binary  bool
	}{
		{name: "xades_baseline"},
		{name: "xades_sha384", options: map[string]string{"algorithm": "SHA384withRSA"}},
		{name: "xades_sha512", options: map[string]string{"algorithm": "SHA512withRSA"}},
		{name: "xades_age18", options: map[string]string{"expPolicy": "firmaage18"}},
		{name: "xades_age19", options: map[string]string{"expPolicy": "firmaage19"}},
		{name: "asic_xml", asic: true},
		{name: "asic_binario", asic: true, binary: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var signed []byte
			if tc.asic {
				data, mime := payload, "application/xml"
				if tc.binary {
					data, mime = []byte{0, 255, 1, 128}, "application/octet-stream"
				}
				value, err := buildASiCXAdESSignatureXML(key, "payload.xml", data, mime, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
				if err != nil {
					t.Fatal(err)
				}
				signed = []byte(value)
			} else {
				var err error
				signed, _, err = buildXAdESBES(domain.SignatureJob{
					Document: domain.Document{Name: "payload.xml", MIMEType: "application/xml", Content: payload},
					Format:   domain.FormatXAdES, Action: domain.ActionSign, Options: tc.options,
				}, key)
				if err != nil {
					t.Fatal(err)
				}
			}
			signedInfo, value, _, err := extractXMLSignatureCore(signed)
			if err != nil {
				t.Fatal(err)
			}
			var info signedInfoForVerify
			if err := xml.Unmarshal(signedInfo, &info); err != nil {
				t.Fatal(err)
			}
			algorithm, err := resolveXAdESAlgorithmOptions(tc.options)
			if err != nil {
				t.Fatal(err)
			}
			checked := 0
			for _, reference := range info.References {
				if !strings.HasPrefix(reference.URI, "#KeyInfo-") && !strings.HasPrefix(reference.URI, "#SignedProperties-") {
					continue
				}
				checked++
				if len(reference.Transforms) != 1 || reference.Transforms[0].Algorithm != algExcC14N {
					t.Errorf("%s no declara exactamente C14N exclusiva: %+v", reference.URI, reference.Transforms)
				}
				canonicalization := algC14N
				if len(reference.Transforms) == 1 {
					canonicalization = reference.Transforms[0].Algorithm
				}
				canonical, err := canonicalizeElementByIDInDocument(signed, strings.TrimPrefix(reference.URI, "#"), canonicalization)
				if err != nil {
					t.Fatal(err)
				}
				if reference.DigestMethod.Algorithm != algorithm.DigestMethod {
					t.Fatalf("DigestMethod inesperado: %s", reference.DigestMethod.Algorithm)
				}
				actual, err := digestBytes(algorithm.Hash, canonical)
				if err != nil {
					t.Fatal(err)
				}
				expected, err := base64.StdEncoding.DecodeString(reference.DigestValue)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(actual, expected) {
					t.Errorf("%s: digest declarado no coincide con canonicalización contextual anunciada", reference.URI)
				}
			}
			if checked != 2 {
				t.Fatalf("referencias internas comprobadas=%d, esperadas=2", checked)
			}
			// También verificar SignedInfo únicamente con el método anunciado,
			// nunca con canonicalizeSignedInfoCandidates ni el fallback legacy.
			canonical, err := canonicalizeElementInDocument(signed, nsXMLDSig, "SignedInfo", info.CanonicalizationMethod.Algorithm)
			if err != nil {
				t.Fatal(err)
			}
			if tc.asic {
				algorithm.Hash = crypto.SHA256
			}
			digest, err := digestBytes(algorithm.Hash, canonical)
			if err != nil {
				t.Fatal(err)
			}
			signature, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := rsa.VerifyPKCS1v15(&private.PublicKey, algorithm.Hash, digest, signature); err != nil {
				t.Fatalf("SignedInfo no verifica con el método anunciado: %v", err)
			}
		})
	}
}
