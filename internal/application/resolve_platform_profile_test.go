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
	"grxfirma/internal/ports"
)

type capabilityProfileProviderMock struct {
	profile ports.CapabilityProfile
	err     error
	calls   int
}

func (m *capabilityProfileProviderMock) Profile(context.Context) (ports.CapabilityProfile, error) {
	m.calls++
	if m.err != nil {
		return ports.CapabilityProfile{}, m.err
	}
	return m.profile, nil
}

func TestResolvePlatformProfile_Exito(t *testing.T) {
	t.Parallel()

	proveedor := &capabilityProfileProviderMock{
		profile: ports.CapabilityProfile{
			HasSecureStorage:    true,
			HasNativeMessaging:  true,
			HasLocalServer:      true,
			HasTemporaryStorage: true,
		},
	}
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	uc := application.NuevoResolvePlatformProfileUseCase(
		proveedor,
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	)

	resultado, err := uc.Ejecutar(context.Background(), application.ResolvePlatformProfileCommand{})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if proveedor.calls != 1 {
		t.Fatalf("calls = %d, want 1", proveedor.calls)
	}
	if !resultado.Profile.HasNativeMessaging {
		t.Fatal("se esperaba HasNativeMessaging = true")
	}
	if len(publicador.eventos) != 2 {
		t.Fatalf("eventos = %d, want 2", len(publicador.eventos))
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestResolvePlatformProfile_FallaProveedor(t *testing.T) {
	t.Parallel()

	proveedor := &capabilityProfileProviderMock{err: errors.New("plataforma no detectable")}
	logger := &loggerMock{}
	uc := application.NuevoResolvePlatformProfileUseCase(
		proveedor,
		application.NuevoAuditUseCase(relojMock{}, logger),
		&publicadorMock{},
	)

	_, err := uc.Ejecutar(context.Background(), application.ResolvePlatformProfileCommand{})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if proveedor.calls != 1 {
		t.Fatalf("calls = %d, want 1", proveedor.calls)
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestResolvePlatformProfile_SinProveedor(t *testing.T) {
	t.Parallel()

	uc := application.NuevoResolvePlatformProfileUseCase(nil, nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.ResolvePlatformProfileCommand{})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}
