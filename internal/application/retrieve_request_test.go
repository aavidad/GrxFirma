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

type transporteRetrieveMock struct {
	retrieveCalls int
	lastSession   domain.ExchangeSession
	data          []byte
	retrieveErr   error
}

func (m *transporteRetrieveMock) Upload(context.Context, domain.ExchangeSession, []byte) error {
	return nil
}

func (m *transporteRetrieveMock) Retrieve(_ context.Context, session domain.ExchangeSession) ([]byte, error) {
	m.retrieveCalls++
	m.lastSession = session
	if m.retrieveErr != nil {
		return nil, m.retrieveErr
	}
	return append([]byte(nil), m.data...), nil
}

func (m *transporteRetrieveMock) SendWait(context.Context, domain.ExchangeSession) error {
	return nil
}

func (m *transporteRetrieveMock) Cancel(context.Context, domain.ExchangeSession) error {
	return nil
}

func TestRetrieveRequest_Exito(t *testing.T) {
	t.Parallel()

	transporte := &transporteRetrieveMock{data: []byte("PETICION")}
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	uc := application.NuevoRetrieveRequestUseCase(
		transporte,
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	)

	resultado, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{
		Session: sesionRemotaPrueba(),
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if transporte.retrieveCalls != 1 {
		t.Fatalf("retrieveCalls = %d, want 1", transporte.retrieveCalls)
	}
	if string(resultado.Data) != "PETICION" {
		t.Fatalf("resultado.Data = %q, want PETICION", string(resultado.Data))
	}
	if len(publicador.eventos) != 2 {
		t.Fatalf("eventos = %d, want 2", len(publicador.eventos))
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestRetrieveRequest_FallaTransporte(t *testing.T) {
	t.Parallel()

	transporte := &transporteRetrieveMock{retrieveErr: errors.New("fallo remoto")}
	logger := &loggerMock{}
	uc := application.NuevoRetrieveRequestUseCase(
		transporte,
		application.NuevoAuditUseCase(relojMock{}, logger),
		&publicadorMock{},
	)

	_, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{
		Session: sesionRemotaPrueba(),
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestRetrieveRequest_Vacio(t *testing.T) {
	t.Parallel()

	transporte := &transporteRetrieveMock{data: nil}
	uc := application.NuevoRetrieveRequestUseCase(transporte, nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{
		Session: sesionRemotaPrueba(),
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}

func TestRetrieveRequest_SesionInvalida(t *testing.T) {
	t.Parallel()

	transporte := &transporteRetrieveMock{}
	uc := application.NuevoRetrieveRequestUseCase(transporte, nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{
		Session: domain.ExchangeSession{},
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if transporte.retrieveCalls != 0 {
		t.Fatalf("retrieveCalls = %d, want 0", transporte.retrieveCalls)
	}
}
