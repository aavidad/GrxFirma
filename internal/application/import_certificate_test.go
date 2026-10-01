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
)

type certificateImporterMock struct {
	lastData     []byte
	lastPassword string
	result       domain.CertificateRef
	err          error
}

func (m *certificateImporterMock) Import(_ context.Context, data []byte, password string) (domain.CertificateRef, error) {
	m.lastData = append([]byte(nil), data...)
	m.lastPassword = password
	if m.err != nil {
		return domain.CertificateRef{}, m.err
	}
	return m.result, nil
}

func TestImportCertificate_Exito(t *testing.T) {
	t.Parallel()

	importador := &certificateImporterMock{
		result: domain.CertificateRef{
			ID:          "cert-1",
			Subject:     "CN=Alice",
			Issuer:      "CN=CA",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "abc123",
		},
	}
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	uc := application.NuevoImportCertificateUseCase(
		importador,
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	)

	resultado, err := uc.Ejecutar(context.Background(), application.ImportCertificateCommand{
		Data:     []byte("P12"),
		Password: "valor-prueba",
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if resultado.Certificate.ID != "cert-1" {
		t.Fatalf("resultado = %+v", resultado)
	}
	if string(importador.lastData) != "P12" || importador.lastPassword != "valor-prueba" {
		t.Fatalf("importador recibio datos incorrectos")
	}
	if len(logger.registros) != 1 || len(publicador.eventos) != 2 {
		t.Fatalf("auditoria/eventos inesperados: registros=%d eventos=%d", len(logger.registros), len(publicador.eventos))
	}
}

func TestImportCertificate_DatosVacios(t *testing.T) {
	t.Parallel()

	uc := application.NuevoImportCertificateUseCase(&certificateImporterMock{}, nil, nil)
	if _, err := uc.Ejecutar(context.Background(), application.ImportCertificateCommand{}); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}

func TestImportCertificate_ErrorImporter(t *testing.T) {
	t.Parallel()

	importador := &certificateImporterMock{err: errors.New("fallo de importacion")}
	logger := &loggerMock{}
	uc := application.NuevoImportCertificateUseCase(
		importador,
		application.NuevoAuditUseCase(relojMock{}, logger),
		nil,
	)
	if _, err := uc.Ejecutar(context.Background(), application.ImportCertificateCommand{
		Data: []byte("P12"),
	}); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestImportCertificate_CertificadoInvalido(t *testing.T) {
	t.Parallel()

	importador := &certificateImporterMock{
		result: domain.CertificateRef{
			ID: "cert-1",
		},
	}
	uc := application.NuevoImportCertificateUseCase(importador, nil, nil)
	if _, err := uc.Ejecutar(context.Background(), application.ImportCertificateCommand{
		Data: []byte("P12"),
	}); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}
