// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestXMLDigest_SHA384YAlgoritmosInvalidos(t *testing.T) {
	payload := []byte("abc")
	t.Run("sha384", func(t *testing.T) {
		actual, err := digestXML(algSHA384, payload)
		if err != nil {
			t.Fatal(err)
		}
		expected := sha512.Sum384(payload)
		if !bytes.Equal(actual, expected[:]) || len(actual) != 48 {
			t.Fatal("digest SHA384 incorrecto")
		}
	})
	for _, algorithm := range []string{"", " ", "urn:qa:unknown", "SHA384", algRSASHA384, algSHA384 + "-unknown"} {
		t.Run("rechaza_"+algorithm, func(t *testing.T) {
			if _, err := digestXML(algorithm, payload); err == nil {
				t.Fatalf("algoritmo de digest no permitido aceptado: %q", algorithm)
			}
		})
	}
}

func TestXAdESVerifier_SHA384YMismaFirmaConPayloadAlterado(t *testing.T) {
	private, cert := certForTest(t, "QA verificación SHA384")
	payload := []byte(`<raiz>prueba SHA384</raiz>`)
	signed, _, err := buildXAdESBES(domain.SignatureJob{
		Document: domain.Document{Name: "payload.xml", MIMEType: "application/xml", Content: payload},
		Format:   domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"algorithm": "SHA384withRSA"},
	}, &LocalSigningKey{Signer: private, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewXAdESVerifier()
	t.Run("firma_valida", func(t *testing.T) {
		result, signers, err := verifier.Verify(context.Background(), domain.Document{Content: signed}, domain.CertificateChain{})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Valid || len(signers) != 1 {
			t.Fatal("SHA384 no aporta integridad válida y un firmante")
		}
	})
	t.Run("payload_alterado", func(t *testing.T) {
		original := []byte(base64.StdEncoding.EncodeToString(payload))
		altered := []byte(base64.StdEncoding.EncodeToString([]byte(`<raiz>otro documento</raiz>`)))
		if bytes.Count(signed, original) != 1 {
			t.Fatal("fixture de contenido embebido ambiguo")
		}
		tampered := bytes.Replace(signed, original, altered, 1)
		_, _, err := verifier.Verify(context.Background(), domain.Document{Content: tampered}, domain.CertificateChain{})
		if err == nil {
			t.Fatal("SHA384 acepta payload alterado sin volver a firmar")
		}
		if !strings.Contains(err.Error(), "digest de referencia no coincide") {
			t.Fatalf("rechazo ajeno al digest manipulado: %v", err)
		}
	})
}
