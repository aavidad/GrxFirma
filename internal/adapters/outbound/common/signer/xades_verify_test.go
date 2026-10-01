// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestXAdESVerifier_VerificaFirmaGeneradaPorV2(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-Verify")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>verificar</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	resultado, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("firmando XAdES: %v", err)
	}

	firmado, err := domain.NewDocument("contrato.xsig", resultado.Data, "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewXAdESVerifier()
	vr, signers, err := verifier.Verify(context.Background(), firmado, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificando XAdES V2: %v", err)
	}
	if !vr.Valid {
		t.Fatalf("se esperaba firma valida, motivo: %s", vr.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante, obtenidos %d", len(signers))
	}
}

func TestXAdESVerifier_VerificaFixtureQA(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_xml_signed.xsig")
	xmlData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("leyendo fixture QA: %v", err)
	}
	doc, err := domain.NewDocument("2_xml_signed.xsig", xmlData, "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewXAdESVerifier()
	vr, signers, err := verifier.Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificando fixture QA: %v", err)
	}
	// La referencia externa de esta muestra antigua sólo coincide con el
	// canonicalizador histórico (véase la regresión de digest específica).
	// Se conserva su diagnóstico y certificado, no un éxito XML estándar.
	requireCompatibility(t, vr)
	if !(vr.Certificate.Status == domain.VerificationStatusValid || (vr.Certificate.Status == domain.VerificationStatusWarning && strings.Contains(vr.Certificate.Reason, "revocación"))) {
		t.Fatalf("el certificado oficial de pruebas debe constar como vigente: %+v", vr.Certificate)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante QA, obtenidos %d", len(signers))
	}
}

func TestXAdESVerifier_FallaSiSeManipulaSignedProperties(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-Tamper")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>manipular</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	resultado, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("firmando XAdES: %v", err)
	}
	manipulada := []byte(strings.Replace(
		string(resultado.Data),
		"<xades:MimeType>application/xml</xades:MimeType>",
		"<xades:MimeType>text/plain</xades:MimeType>",
		1,
	))
	firmado, err := domain.NewDocument("contrato.xsig", manipulada, "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewXAdESVerifier()
	if _, _, err := verifier.Verify(context.Background(), firmado, domain.CertificateChain{}); err == nil {
		t.Fatal("se esperaba error al manipular SignedProperties")
	}
}
