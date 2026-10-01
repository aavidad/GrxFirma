// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestXMLDSigDetached_SignYVerifica(t *testing.T) {
	priv, cert := certForTest(t, "XMLDSig-RSA")
	engine := NewXMLDSigDetached()
	verifier := NewXMLDSigVerifier()

	doc, err := domain.NewDocument("muestra.xml", []byte("<doc><name>xmlsig</name></doc>"), "application/xml")
	if err != nil {
		t.Fatalf("documento: %v", err)
	}
	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   formatXMLDSig,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "clave-xmldsig", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("firmando XMLdSig: %v", err)
	}
	if result.Format != formatXMLDSig {
		t.Fatalf("formato = %q, want %q", result.Format, formatXMLDSig)
	}
	xmlOut := string(result.Data)
	if !strings.Contains(xmlOut, "<ds:SignedInfo") {
		t.Fatal("la salida XMLdSig debe contener SignedInfo")
	}
	if strings.Contains(xmlOut, "xades:QualifyingProperties") {
		t.Fatal("la salida XMLdSig no debe contener XAdES")
	}
	verifyResult, signers, err := verifier.Verify(context.Background(), domain.Document{
		Name:     "muestra_firmada.dsig",
		MIMEType: "application/xmldsig+xml",
		Content:  result.Data,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificando XMLdSig: %v", err)
	}
	if !verifyResult.Valid {
		t.Fatalf("la firma XMLdSig debería ser válida: %s", verifyResult.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante XMLdSig, obtenidos %d", len(signers))
	}
}

func TestMultiVerifier_VerificaXMLDSigPorExtension(t *testing.T) {
	priv, cert := certForTest(t, "XMLDSig-Multi")
	engine := NewXMLDSigDetached()
	doc, err := domain.NewDocument("muestra.xml", []byte("<root><a>1</a></root>"), "application/xml")
	if err != nil {
		t.Fatalf("documento: %v", err)
	}
	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   formatXMLDSig,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "clave-xmldsig", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("firmando XMLdSig: %v", err)
	}
	v := NewMultiVerifier()
	verification, _, err := v.Verify(context.Background(), domain.Document{
		Name:     "muestra_firmada.dsig",
		MIMEType: "application/xmldsig+xml",
		Content:  result.Data,
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificando XMLdSig por multiverifier: %v", err)
	}
	if !verification.Valid {
		t.Fatalf("la firma XMLdSig debería ser válida: %s", verification.Reason)
	}
}
