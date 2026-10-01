// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nativehost

import (
	"context"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type metricsMock struct {
	op     string
	status string
	count  int
}

func (m *metricsMock) RecordSign(context.Context, string, string, time.Duration) {}

func (m *metricsMock) RecordCertificateSource(context.Context, string) {}

func (m *metricsMock) RecordProtocolRequest(_ context.Context, op string, status string, _ time.Duration) {
	m.op = op
	m.status = status
	m.count++
}

func TestProcessRegistraMetricasDeProtocolo(t *testing.T) {
	t.Parallel()

	metricas := &metricsMock{}
	adapter := New(nil, signMock{result: application.SignResult{
		Result: domain.SignatureResult{
			Format:    domain.FormatCAdES,
			Algorithm: "SHA256withRSA",
			Data:      []byte("firma"),
		},
		CertificateUsed: domain.CertificateRef{ID: "cert-1"},
	}}, nil, nil).WithMetrics(metricas)

	_, err := adapter.Process(context.Background(), "", []byte(`{"requestId":"m1","action":"ping"}`))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if metricas.count != 1 {
		t.Fatalf("metricas.count = %d, want 1", metricas.count)
	}
	if metricas.op != "ping" || metricas.status != "ok" {
		t.Fatalf("metricas = %+v", metricas)
	}
}
