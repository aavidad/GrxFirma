// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain_test

import (
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestNewDocument_EmptyContent(t *testing.T) {
	_, err := domain.NewDocument("test.pdf", nil, "application/pdf")
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestNewDocument_Valid(t *testing.T) {
	doc, err := domain.NewDocument("test.pdf", []byte("content"), "application/pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Size() != 7 {
		t.Fatalf("expected size 7, got %d", doc.Size())
	}
}

func TestCertificateRef_IsExpired(t *testing.T) {
	ref := domain.CertificateRef{
		ID:          "abc",
		Fingerprint: "deadbeef",
		NotAfter:    time.Now().Add(-time.Hour),
	}
	if !ref.IsExpired(time.Now()) {
		t.Fatal("expected certificate to be expired")
	}
}

func TestCertificateRef_Validate_MissingID(t *testing.T) {
	ref := domain.CertificateRef{Fingerprint: "deadbeef"}
	if err := ref.Validate(); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestSignatureFormat_Validate(t *testing.T) {
	for _, f := range []domain.SignatureFormat{domain.FormatCAdES, domain.FormatXAdES, domain.FormatPAdES} {
		if err := f.Validate(); err != nil {
			t.Fatalf("valid format %q gave error: %v", f, err)
		}
	}
	if err := domain.SignatureFormat("XMLdSig").Validate(); err != nil {
		t.Fatalf("generic format should be accepted by domain: %v", err)
	}
	if err := domain.SignatureFormat("").Validate(); err == nil {
		t.Fatal("expected error for empty format")
	}
}

func TestSignatureAction_Validate(t *testing.T) {
	for _, a := range []domain.SignatureAction{domain.ActionSign, domain.ActionCoSign, domain.ActionCounterSign} {
		if err := a.Validate(); err != nil {
			t.Fatalf("valid action %q gave error: %v", a, err)
		}
	}
	if err := domain.SignatureAction("delete").Validate(); err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestSignatureJob_Validate_EmptyDocument(t *testing.T) {
	job := domain.SignatureJob{
		Format: domain.FormatCAdES,
		Action: domain.ActionSign,
	}
	if err := job.Validate(); err == nil {
		t.Fatal("expected error for empty document")
	}
}

func TestBatchJob_Validate_Empty(t *testing.T) {
	b := domain.BatchJob{}
	if err := b.Validate(); err == nil {
		t.Fatal("expected error for empty batch")
	}
}

func TestTrustDecision_IsAllowed(t *testing.T) {
	d := domain.TrustDecision{Origin: "example.com", Status: domain.TrustAllowed}
	if !d.IsAllowed() {
		t.Fatal("expected allowed")
	}
	d.Status = domain.TrustDenied
	if d.IsAllowed() {
		t.Fatal("expected not allowed")
	}
}

func TestExchangeSession_Validate_MissingRequestID(t *testing.T) {
	s := domain.ExchangeSession{
		UploadEndpoint:   "http://srv/upload",
		RetrieveEndpoint: "http://srv/retrieve",
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for missing RequestID")
	}
}

func TestExchangeSession_WithState(t *testing.T) {
	s := domain.ExchangeSession{
		RequestID:        "req-1",
		State:            domain.SessionActive,
		UploadEndpoint:   "http://srv/upload",
		RetrieveEndpoint: "http://srv/retrieve",
	}
	s2 := s.WithState(domain.SessionCancelled)
	if s2.State != domain.SessionCancelled {
		t.Fatal("expected cancelled state")
	}
	// original no debe mutar
	if s.State != domain.SessionActive {
		t.Fatal("original session state mutated")
	}
}
