// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
)

func TestVerifySignature_ExitoCAdES(t *testing.T) {
	logger := &loggerMock{}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{
			chain: domain.CertificateChain{Certificates: []domain.CertificateRef{certPrueba()}},
		},
		&verifierMock{
			result:  domain.VerificationResult{Valid: true, Reason: "CAdES valido"},
			signers: []domain.CertificateRef{certPrueba()},
		},
		auditor,
	)

	res, err := uc.Ejecutar(context.Background(), application.VerifyCommand{
		SignedDocument: docFirmadoPrueba(),
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !res.Verification.Valid {
		t.Fatal("la verificacion deberia ser valida")
	}
	if len(res.Firmantes) != 1 {
		t.Fatalf("se esperaba un firmante, obtenidos %d", len(res.Firmantes))
	}
	if len(logger.registros) != 1 {
		t.Fatalf("se esperaba un registro de auditoria, obtenidos %d", len(logger.registros))
	}
}

func TestVerifySignature_ErrorAnclas(t *testing.T) {
	logger := &loggerMock{}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{err: errors.New("trust store roto")},
		&verifierMock{},
		auditor,
	)

	_, err := uc.Ejecutar(context.Background(), application.VerifyCommand{
		SignedDocument: docFirmadoPrueba(),
	})
	if err == nil {
		t.Fatal("se esperaba error al obtener anclas")
	}
}

func TestVerifySignature_ErrorVerifier(t *testing.T) {
	logger := &loggerMock{}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{
			chain: domain.CertificateChain{Certificates: []domain.CertificateRef{certPrueba()}},
		},
		&verifierMock{err: errors.New("firma CAdES corrupta")},
		auditor,
	)

	_, err := uc.Ejecutar(context.Background(), application.VerifyCommand{
		SignedDocument: docFirmadoPrueba(),
	})
	if err == nil {
		t.Fatal("se esperaba error del verificador")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("se esperaba un registro de auditoria, obtenidos %d", len(logger.registros))
	}
}

type trustAnchorMock struct {
	chain domain.CertificateChain
	err   error
}

func (m *trustAnchorMock) Anchors(context.Context) (domain.CertificateChain, error) {
	return m.chain, m.err
}

type verifierMock struct {
	result  domain.VerificationResult
	signers []domain.CertificateRef
	err     error
}

func (m *verifierMock) Verify(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return m.result, m.signers, m.err
}

type verifierDetachedMock struct {
	result  domain.VerificationResult
	signers []domain.CertificateRef
	err     error
}

func (m *verifierDetachedMock) Verify(context.Context, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return domain.VerificationResult{}, nil, errors.New("no debería llamarse a Verify normal en modo detached")
}

func (m *verifierDetachedMock) VerifyDetached(context.Context, domain.Document, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return m.result, m.signers, m.err
}

func docFirmadoPrueba() domain.Document {
	doc, _ := domain.NewDocument("firma.csig", []byte("contenido-firmado"), "application/pkcs7-signature")
	return doc
}

func TestVerifySignature_PropagaContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger := &loggerMock{}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{err: context.Canceled},
		&verifierMock{},
		auditor,
	)

	_, err := uc.Ejecutar(ctx, application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}

func TestVerifySignature_DetachedConOriginal(t *testing.T) {
	logger := &loggerMock{}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{
			chain: domain.CertificateChain{Certificates: []domain.CertificateRef{certPrueba()}},
		},
		&verifierDetachedMock{
			result:  domain.VerificationResult{Valid: true, Reason: "detached valido"},
			signers: []domain.CertificateRef{certPrueba()},
		},
		auditor,
	)

	original, _ := domain.NewDocument("original.bin", []byte("original"), "application/octet-stream")
	res, err := uc.Ejecutar(context.Background(), application.VerifyCommand{
		SignedDocument:   docFirmadoPrueba(),
		OriginalDocument: &original,
	})
	if err != nil {
		t.Fatalf("no se esperaba error detached: %v", err)
	}
	if !res.Verification.Valid {
		t.Fatalf("la verificación detached debería ser válida: %s", res.Verification.Reason)
	}
}

var _ = time.UTC

func TestVerifyBlockedLegacyReturnsUnevaluated(t *testing.T) {
	block := &excepciones.Block{ID: "sha1_leer", BlockKey: "excepciones.sha1_leer.bloqueo", SettingKey: "excepciones.sha1_leer.ajuste"}
	uc := application.NuevoVerifySignatureUseCase(&trustAnchorMock{}, &verifierMock{err: block}, nil)
	out, err := uc.Execute(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil || out.Verification.Valid || out.Verification.Integrity.Status != domain.VerificationStatusUnknown || excepciones.FromText(out.Verification.Reason) == nil {
		t.Fatalf("bloqueo perdido: valid=%v integrity=%s err=%v", out.Verification.Valid, out.Verification.Integrity.Status, err)
	}
}

type anclasConAvisoPrevio struct{}

func (anclasConAvisoPrevio) Anchors(ctx context.Context) (domain.CertificateChain, error) {
	domain.AnotarAviso(ctx, "excepciones.anclas_locales.aviso")
	return domain.CertificateChain{}, nil
}
func TestVerifyBlockedLegacyPreservesAnchorWarning(t *testing.T) {
	block := &excepciones.Block{ID: "sha1_leer", BlockKey: "excepciones.sha1_leer.bloqueo", SettingKey: "excepciones.sha1_leer.ajuste"}
	uc := application.NuevoVerifySignatureUseCase(anclasConAvisoPrevio{}, &verifierMock{err: block}, nil)
	out, err := uc.Execute(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, w := range out.Verification.Warnings {
		if w == "excepciones.anclas_locales.aviso" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("el bloqueo perdió el aviso de anclas locales: %v", out.Verification.Warnings)
	}
}
