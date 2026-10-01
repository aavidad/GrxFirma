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
)

type trustPolicyMock struct {
	allowed []string
	denied  []string
	removed []string
	err     error
}

func (m *trustPolicyMock) Evaluate(context.Context, string) (domain.TrustDecision, error) {
	return domain.TrustDecision{}, nil
}

func (m *trustPolicyMock) Allow(_ context.Context, origin string) error {
	if m.err != nil {
		return m.err
	}
	m.allowed = append(m.allowed, origin)
	return nil
}

func (m *trustPolicyMock) Deny(_ context.Context, origin string) error {
	if m.err != nil {
		return m.err
	}
	m.denied = append(m.denied, origin)
	return nil
}

func (m *trustPolicyMock) Remove(_ context.Context, origin string) error {
	if m.err != nil {
		return m.err
	}
	m.removed = append(m.removed, origin)
	return nil
}

func TestManageTrustedDomain_Allow(t *testing.T) {
	t.Parallel()

	politica := &trustPolicyMock{}
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	uc := application.NuevoManageTrustedDomainUseCase(
		politica,
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	)

	resultado, err := uc.Ejecutar(context.Background(), application.ManageTrustedDomainCommand{
		Origin: "https://firma.example",
		Action: application.TrustActionAllow,
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if resultado.Action != application.TrustActionAllow || resultado.Origin != "https://firma.example" {
		t.Fatalf("resultado = %+v", resultado)
	}
	if len(politica.allowed) != 1 {
		t.Fatalf("allowed = %v, want 1 elemento", politica.allowed)
	}
	if len(logger.registros) != 1 || len(publicador.eventos) != 1 {
		t.Fatalf("auditoria/eventos inesperados: registros=%d eventos=%d", len(logger.registros), len(publicador.eventos))
	}
}

func TestManageTrustedDomain_Deny(t *testing.T) {
	t.Parallel()

	politica := &trustPolicyMock{}
	uc := application.NuevoManageTrustedDomainUseCase(politica, nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.ManageTrustedDomainCommand{
		Origin: "https://firma.example",
		Action: application.TrustActionDeny,
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if len(politica.denied) != 1 {
		t.Fatalf("denied = %v, want 1 elemento", politica.denied)
	}
}

func TestManageTrustedDomain_Remove(t *testing.T) {
	t.Parallel()

	politica := &trustPolicyMock{}
	uc := application.NuevoManageTrustedDomainUseCase(politica, nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.ManageTrustedDomainCommand{
		Origin: "https://firma.example",
		Action: application.TrustActionRemove,
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if len(politica.removed) != 1 {
		t.Fatalf("removed = %v, want 1 elemento", politica.removed)
	}
}

func TestManageTrustedDomain_ErrorPolitica(t *testing.T) {
	t.Parallel()

	politica := &trustPolicyMock{err: errors.New("fallo de almacenamiento")}
	logger := &loggerMock{}
	uc := application.NuevoManageTrustedDomainUseCase(
		politica,
		application.NuevoAuditUseCase(relojMock{}, logger),
		nil,
	)

	_, err := uc.Ejecutar(context.Background(), application.ManageTrustedDomainCommand{
		Origin: "https://firma.example",
		Action: application.TrustActionAllow,
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestManageTrustedDomain_AccionNoSoportada(t *testing.T) {
	t.Parallel()

	politica := &trustPolicyMock{}
	uc := application.NuevoManageTrustedDomainUseCase(politica, nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.ManageTrustedDomainCommand{
		Origin: "https://firma.example",
		Action: application.TrustAction("otro"),
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}
