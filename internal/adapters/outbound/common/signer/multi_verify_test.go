// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

type verifierEngineMock struct {
	verifyFn func(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
}

func (m *verifierEngineMock) Verify(ctx context.Context, doc domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if m.verifyFn == nil {
		return domain.VerificationResult{}, nil, nil
	}
	return m.verifyFn(ctx, doc, anchors)
}

type detachedVerifierEngineMock struct {
	verifyFn         func(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
	verifyDetachedFn func(context.Context, domain.Document, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
}

func (m *detachedVerifierEngineMock) Verify(ctx context.Context, doc domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if m.verifyFn == nil {
		return domain.VerificationResult{}, nil, nil
	}
	return m.verifyFn(ctx, doc, anchors)
}

func (m *detachedVerifierEngineMock) VerifyDetached(ctx context.Context, signed domain.Document, original domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if m.verifyDetachedFn == nil {
		return domain.VerificationResult{}, nil, nil
	}
	return m.verifyDetachedFn(ctx, signed, original, anchors)
}

func TestMultiVerifier_SeleccionaPAdESPorMagicHeader(t *testing.T) {
	v := commonsigner.NewMultiVerifier()
	doc, _ := domain.NewDocument("firmado.bin", []byte("%PDF-1.7 resto"), "application/octet-stream")
	_, _, err := v.Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil {
		t.Fatal("se esperaba error posterior de verificación, no de detección")
	}
	if err != nil && err.Error() == `no se pudo detectar el formato de firma para "firmado.bin"` {
		t.Fatal("la detección de PAdES por cabecera PDF no debería fallar")
	}
}

func TestMultiVerifier_SeleccionaCAdESPorExtension(t *testing.T) {
	v := commonsigner.NewMultiVerifier()
	doc, _ := domain.NewDocument("firma.csig", []byte{0x30, 0x82, 0x01, 0x00}, "application/octet-stream")
	_, _, err := v.Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil {
		t.Fatal("se esperaba error posterior de verificación, no de detección")
	}
	if err != nil && err.Error() == `no se pudo detectar el formato de firma para "firma.csig"` {
		t.Fatal("la detección de CAdES por extensión no debería fallar")
	}
}

func TestMultiVerifier_RechazaFormatoDesconocido(t *testing.T) {
	v := commonsigner.NewMultiVerifier()
	doc, _ := domain.NewDocument("firma.bin", []byte("nada"), "application/octet-stream")
	_, _, err := v.Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil {
		t.Fatal("se esperaba error de formato desconocido")
	}
}

func TestMultiVerifier_VerificaXAdESPorExtension(t *testing.T) {
	v := commonsigner.NewMultiVerifier()
	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_xml_signed.xsig")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("leyendo fixture XAdES QA: %v", err)
	}
	doc, _ := domain.NewDocument("2_xml_signed.xsig", data, "application/xml")
	result, signers, err := v.Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificando XAdES por multiverifier: %v", err)
	}
	if result.Valid || result.Integrity.Status != domain.VerificationStatusWarning {
		t.Fatalf("el dispatcher debe conservar la compatibilidad histórica, no éxito estándar: %+v", result)
	}
	compatibility := false
	for _, evidence := range result.Evidence {
		compatibility = compatibility || evidence.Type == "xml.canonicalization.compatibility"
	}
	if !compatibility {
		t.Fatal("se perdió la evidencia tipada de compatibilidad")
	}
	if !(result.Certificate.Status == domain.VerificationStatusValid || (result.Certificate.Status == domain.VerificationStatusWarning && strings.Contains(result.Certificate.Reason, "revocación"))) {
		t.Fatalf("el certificado oficial de pruebas debe marcarse vigente: %+v", result.Certificate)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante XAdES, obtenidos %d", len(signers))
	}
}

func TestMultiVerifier_VerifyDetached_PAdESIgnoraOriginalYVerificaEmbedded(t *testing.T) {
	t.Helper()
	calledVerify := false
	calledDetached := false

	pades := &detachedVerifierEngineMock{
		verifyFn: func(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
			calledVerify = true
			return domain.VerificationResult{Valid: true, Reason: "firma PAdES válida"}, []domain.CertificateRef{{ID: "firmante"}}, nil
		},
		verifyDetachedFn: func(context.Context, domain.Document, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
			calledDetached = true
			return domain.VerificationResult{}, nil, nil
		},
	}

	v := commonsigner.NewMultiVerifierWithEngines(
		&verifierEngineMock{},
		&detachedVerifierEngineMock{},
		&verifierEngineMock{},
		pades,
		&verifierEngineMock{},
		&verifierEngineMock{},
		&verifierEngineMock{},
		&verifierEngineMock{},
	)

	signed, _ := domain.NewDocument("firmado.pdf", []byte("%PDF-1.7 firmado"), "application/pdf")
	original, _ := domain.NewDocument("original.pdf", []byte("%PDF-1.7 original"), "application/pdf")

	result, signers, err := v.VerifyDetached(context.Background(), signed, original, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("VerifyDetached(PAdES) error inesperado: %v", err)
	}
	if !calledVerify {
		t.Fatal("PAdES con original aportado debería verificar usando Verify embebido")
	}
	if calledDetached {
		t.Fatal("PAdES no debería invocar VerifyDetached")
	}
	if !result.Valid {
		t.Fatalf("resultado PAdES inesperado: %#v", result)
	}
	if len(signers) != 1 {
		t.Fatalf("firmantes inesperados: %d", len(signers))
	}
	foundIgnoredDetail := false
	for _, detail := range result.Details {
		if detail == "original_aportado_ignorado_en_pades" {
			foundIgnoredDetail = true
			break
		}
	}
	if !foundIgnoredDetail {
		t.Fatalf("se esperaba detalle de original ignorado en PAdES: %#v", result.Details)
	}
}
