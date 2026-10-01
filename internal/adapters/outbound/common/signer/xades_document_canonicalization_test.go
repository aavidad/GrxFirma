// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/xml"
	"testing"

	"grxfirma/internal/domain"
)

func TestXAdESDocumento_RespetaCanonicalizacionDeclarada(t *testing.T) {
	private, cert := certForTest(t, "QA canonicalización documento")
	// El prefijo no utilizado distingue inclusiva de exclusiva: un simple
	// ida/vuelta con XML sin namespaces no descubre el error histórico AGE.
	payload := []byte(`<raiz xmlns="urn:qa:payload" xmlns:unused="urn:qa:unused"><dato>prueba</dato></raiz>`)
	for _, policy := range []string{"", "firmaage18", "firmaage19"} {
		t.Run("politica_"+policy, func(t *testing.T) {
			signed, _, err := buildXAdESBES(domain.SignatureJob{
				Document: domain.Document{Name: "payload.xml", MIMEType: "application/xml", Content: payload},
				Format:   domain.FormatXAdES, Action: domain.ActionSign,
				Options: map[string]string{"expPolicy": policy},
			}, &LocalSigningKey{Signer: private, Certificate: cert})
			if err != nil {
				t.Fatal(err)
			}
			infoXML, _, _, err := extractXMLSignatureCore(signed)
			if err != nil {
				t.Fatal(err)
			}
			var info signedInfoForVerify
			if err := xml.Unmarshal(infoXML, &info); err != nil {
				t.Fatal(err)
			}
			if len(info.References) != 3 {
				t.Fatal("número inesperado de referencias")
			}
			ref := info.References[0]
			wantAlgorithm, wantURI := algExcC14N, "payload.xml"
			if policy != "" {
				wantAlgorithm, wantURI = algC14N, "#CONTENT-1"
			}
			if ref.URI != wantURI || len(ref.Transforms) != 1 || ref.Transforms[0].Algorithm != wantAlgorithm {
				t.Fatalf("cambió el contrato de referencia: %+v", ref)
			}
			var canonical []byte
			if policy != "" {
				canonical, err = canonicalizeElementByIDInDocument(signed, "CONTENT-1", wantAlgorithm)
			} else {
				canonical, err = canonicalizeXML(payload, wantAlgorithm)
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, err := digestBytes(crypto.SHA256, canonical)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := base64.StdEncoding.DecodeString(ref.DigestValue)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, expected) {
				t.Fatal("digest del documento no coincide con C14N contextual declarada; no usar fallback")
			}
		})
	}
}
