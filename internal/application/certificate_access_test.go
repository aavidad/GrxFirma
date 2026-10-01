// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type certificateAccessMock struct {
	importTarget string
	importData   []byte
}

func (m *certificateAccessMock) Options(context.Context) (ports.CertificateAccessOptions, error) {
	return ports.CertificateAccessOptions{DetectedBrowser: "firefox"}, nil
}
func (m *certificateAccessMock) OpenManager(context.Context, string) error { return nil }
func (m *certificateAccessMock) Import(_ context.Context, target string, data []byte, _ string) error {
	m.importTarget = target
	m.importData = append([]byte(nil), data...)
	return nil
}

type temporaryStoreMock struct {
	loaded  bool
	removed string
	cleared bool
}

func (m *temporaryStoreMock) Load(context.Context, []byte, string) (domain.CertificateRef, error) {
	m.loaded = true
	return domain.CertificateRef{ID: "tmp", Fingerprint: "aa"}, nil
}
func (m *temporaryStoreMock) List(context.Context) ([]domain.CertificateRef, error) { return nil, nil }
func (m *temporaryStoreMock) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return nil, errors.New("no usado")
}
func (m *temporaryStoreMock) Remove(id string) { m.removed = id }
func (m *temporaryStoreMock) Clear()           { m.cleared = true }

func TestCertificateAccessImportRequiresExplicitTarget(t *testing.T) {
	mock := &certificateAccessMock{}
	uc := application.NuevoCertificateAccessUseCase(mock)
	if err := uc.Import(context.Background(), application.ImportCertificateToStoreCommand{
		Data: []byte("p12"),
	}); err == nil {
		t.Fatal("Import() debía exigir destino explícito")
	}
	if err := uc.Import(context.Background(), application.ImportCertificateToStoreCommand{
		TargetID: "nss:test", Data: []byte("p12"),
	}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if mock.importTarget != "nss:test" || string(mock.importData) != "p12" {
		t.Fatalf("importación inesperada: %#v", mock)
	}
}

func TestTemporaryCertificateUseAndRemove(t *testing.T) {
	store := &temporaryStoreMock{}
	uc := application.NuevoTemporaryCertificateUseCase(store)
	result, err := uc.Use(context.Background(), application.UseTemporaryCertificateCommand{Data: []byte("pem")})
	if err != nil || result.Certificate.ID != "tmp" || !store.loaded {
		t.Fatalf("Use() = %#v, %v", result, err)
	}
	if err := uc.Remove(context.Background(), application.RemoveTemporaryCertificateCommand{CertificateID: "tmp"}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if store.removed != "tmp" {
		t.Fatalf("Remove() id = %q", store.removed)
	}
	if err := uc.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if !store.cleared {
		t.Fatal("Clear() no vació el almacén temporal")
	}
}
