// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

type xadesTSAMock struct {
	hash     []byte
	hashAlgo crypto.Hash
	token    []byte
}

func (m *xadesTSAMock) RequestTimestamp(_ context.Context, hash []byte, hashAlgo crypto.Hash) ([]byte, error) {
	m.hash = append([]byte(nil), hash...)
	m.hashAlgo = hashAlgo
	return append([]byte(nil), m.token...), nil
}

func TestSignerXAdEST_AñadeSignatureTimeStampYUsaTSA(t *testing.T) {
	t.Parallel()

	priv, cert := certForTest(t, "XAdES-T")
	key := &LocalSigningKey{
		ID:          "clave-xades-t",
		Signer:      priv,
		Certificate: cert,
	}
	doc, err := domain.NewDocument("contrato.xml", []byte(`<Contrato><Importe>42</Importe></Contrato>`), "application/xml")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	tsa := &xadesTSAMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}
	engine := NewSignerXAdEST(NewXAdESBESDetached(), tsa)

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, key)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	xmlOut := string(result.Data)
	if !strings.Contains(xmlOut, "<xades:UnsignedProperties>") {
		t.Fatalf("la firma no contiene UnsignedProperties:\n%s", xmlOut)
	}
	if !strings.Contains(xmlOut, "<xades:SignatureTimeStamp>") {
		t.Fatalf("la firma no contiene SignatureTimeStamp:\n%s", xmlOut)
	}
	if !strings.Contains(xmlOut, `<ds:CanonicalizationMethod xmlns:ds="`+nsXMLDSig+`" Algorithm="`+algExcC14N+`"/>`) {
		t.Fatalf("la firma no contiene CanonicalizationMethod esperada:\n%s", xmlOut)
	}
	expectedTokenB64 := base64.StdEncoding.EncodeToString(tsa.token)
	if !strings.Contains(xmlOut, "<xades:EncapsulatedTimeStamp>"+expectedTokenB64+"</xades:EncapsulatedTimeStamp>") {
		t.Fatalf("la firma no contiene EncapsulatedTimeStamp esperado:\n%s", xmlOut)
	}
	if tsa.hashAlgo != crypto.SHA256 {
		t.Fatalf("hash algo TSA = %v, want SHA256", tsa.hashAlgo)
	}

	signatureValueXML, err := extractXMLLiteralElement(result.Data, "ds:SignatureValue")
	if err != nil {
		t.Fatalf("extractXMLLiteralElement(SignatureValue) error = %v", err)
	}
	c14n, err := exclusiveC14N(string(signatureValueXML))
	if err != nil {
		t.Fatalf("exclusiveC14N(SignatureValue) error = %v", err)
	}
	expectedHash := sha256.Sum256(c14n)
	if string(tsa.hash) != string(expectedHash[:]) {
		t.Fatalf("hash TSA inesperado")
	}

	verifier := NewXAdESVerifier()
	verifyResult, signers, err := verifier.Verify(context.Background(), domain.Document{
		Name:     "firma.xsig",
		Content:  result.Data,
		MIMEType: "application/xml",
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !verifyResult.Valid {
		t.Fatalf("la firma XAdES-T debería seguir siendo válida: %s", verifyResult.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("signers = %d, want 1", len(signers))
	}
}
