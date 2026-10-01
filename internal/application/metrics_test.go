// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type metricsRecorderMock struct {
	signFormat string
	signStatus string
	signCount  int
}

func (m *metricsRecorderMock) RecordSign(_ context.Context, format string, status string, _ time.Duration) {
	m.signFormat = format
	m.signStatus = status
	m.signCount++
}

func (m *metricsRecorderMock) RecordCertificateSource(context.Context, string) {}

func (m *metricsRecorderMock) RecordProtocolRequest(context.Context, string, string, time.Duration) {}

func TestSignDocument_RegistraMetricas(t *testing.T) {
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	metricas := &metricsRecorderMock{}
	resultado := domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma"), Algorithm: "RSA"}

	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorMock{resultado: resultado},
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	).WithMetrics(metricas)

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if metricas.signCount != 1 || metricas.signFormat != "CAdES" || metricas.signStatus != "ok" {
		t.Fatalf("metricas = %+v", metricas)
	}
}
