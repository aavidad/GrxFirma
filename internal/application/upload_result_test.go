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

type transporteResultadoMock struct {
	uploadCalls int
	lastSession domain.ExchangeSession
	lastData    []byte
	uploadErr   error
}

func (m *transporteResultadoMock) Upload(_ context.Context, session domain.ExchangeSession, data []byte) error {
	m.uploadCalls++
	m.lastSession = session
	m.lastData = append([]byte(nil), data...)
	return m.uploadErr
}

func (m *transporteResultadoMock) Retrieve(context.Context, domain.ExchangeSession) ([]byte, error) {
	return nil, nil
}

func (m *transporteResultadoMock) SendWait(context.Context, domain.ExchangeSession) error {
	return nil
}

func (m *transporteResultadoMock) Cancel(context.Context, domain.ExchangeSession) error {
	return nil
}

func sesionRemotaPrueba() domain.ExchangeSession {
	return domain.ExchangeSession{
		RequestID:        "req-1",
		SessionKey:       "clave",
		UploadEndpoint:   "https://upload.example",
		RetrieveEndpoint: "https://retrieve.example",
		State:            domain.SessionActive,
	}
}

func TestUploadResult_Exito(t *testing.T) {
	t.Parallel()

	transporte := &transporteResultadoMock{}
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	uc := application.NuevoUploadResultUseCase(
		transporte,
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	)

	err := uc.Ejecutar(context.Background(), application.UploadResultCommand{
		Session: sesionRemotaPrueba(),
		Data:    []byte("resultado"),
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if transporte.uploadCalls != 1 {
		t.Fatalf("uploadCalls = %d, want 1", transporte.uploadCalls)
	}
	if string(transporte.lastData) != "resultado" {
		t.Fatalf("lastData = %q, want resultado", string(transporte.lastData))
	}
	if len(publicador.eventos) != 2 {
		t.Fatalf("eventos = %d, want 2", len(publicador.eventos))
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestUploadResult_FallaTransporte(t *testing.T) {
	t.Parallel()

	transporte := &transporteResultadoMock{uploadErr: errors.New("fallo remoto")}
	logger := &loggerMock{}
	uc := application.NuevoUploadResultUseCase(
		transporte,
		application.NuevoAuditUseCase(relojMock{}, logger),
		&publicadorMock{},
	)

	err := uc.Ejecutar(context.Background(), application.UploadResultCommand{
		Session: sesionRemotaPrueba(),
		Data:    []byte("resultado"),
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestUploadResult_SesionInvalida(t *testing.T) {
	t.Parallel()

	transporte := &transporteResultadoMock{}
	uc := application.NuevoUploadResultUseCase(transporte, nil, nil)

	err := uc.Ejecutar(context.Background(), application.UploadResultCommand{
		Session: domain.ExchangeSession{},
		Data:    []byte("resultado"),
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if transporte.uploadCalls != 0 {
		t.Fatalf("uploadCalls = %d, want 0", transporte.uploadCalls)
	}
}

func TestUploadResult_DatosVacios(t *testing.T) {
	t.Parallel()

	transporte := &transporteResultadoMock{}
	uc := application.NuevoUploadResultUseCase(transporte, nil, nil)

	err := uc.Ejecutar(context.Background(), application.UploadResultCommand{
		Session: sesionRemotaPrueba(),
		Data:    nil,
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if transporte.uploadCalls != 0 {
		t.Fatalf("uploadCalls = %d, want 0", transporte.uploadCalls)
	}
}
