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
)

type desktopNotificationMock struct {
	err    error
	calls  int
	titles []string
}

func (m *desktopNotificationMock) Notify(_ context.Context, title, body string) error {
	m.calls++
	m.titles = append(m.titles, title+"|"+body)
	return m.err
}

type mobileNotificationMock struct {
	err    error
	calls  int
	titles []string
}

func (m *mobileNotificationMock) Push(_ context.Context, title, body string) error {
	m.calls++
	m.titles = append(m.titles, title+"|"+body)
	return m.err
}

func TestNotifyUser_DesktopDisponible(t *testing.T) {
	t.Parallel()

	desktop := &desktopNotificationMock{}
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	uc := application.NuevoNotifyUserUseCase(
		desktop,
		nil,
		application.NuevoAuditUseCase(relojMock{}, logger),
		publicador,
	)

	resultado, err := uc.Ejecutar(context.Background(), application.NotifyUserCommand{
		Title: "Firma completada",
		Body:  "Documento firmado",
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if resultado.Channel != "desktop" {
		t.Fatalf("resultado.Channel = %q, want desktop", resultado.Channel)
	}
	if desktop.calls != 1 || len(logger.registros) != 1 || len(publicador.eventos) != 2 {
		t.Fatalf("estado inesperado: calls=%d registros=%d eventos=%d", desktop.calls, len(logger.registros), len(publicador.eventos))
	}
}

func TestNotifyUser_CaeADesktopYUsaMobile(t *testing.T) {
	t.Parallel()

	desktop := &desktopNotificationMock{err: errors.New("desktop down")}
	mobile := &mobileNotificationMock{}
	uc := application.NuevoNotifyUserUseCase(desktop, mobile, nil, nil)

	resultado, err := uc.Ejecutar(context.Background(), application.NotifyUserCommand{
		Title: "Aviso",
		Body:  "Canal alternativo",
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if resultado.Channel != "mobile" {
		t.Fatalf("resultado.Channel = %q, want mobile", resultado.Channel)
	}
	if desktop.calls != 1 || mobile.calls != 1 {
		t.Fatalf("calls inesperadas: desktop=%d mobile=%d", desktop.calls, mobile.calls)
	}
}

func TestNotifyUser_FallanTodosLosCanales(t *testing.T) {
	t.Parallel()

	logger := &loggerMock{}
	uc := application.NuevoNotifyUserUseCase(
		&desktopNotificationMock{err: errors.New("desktop down")},
		&mobileNotificationMock{err: errors.New("mobile down")},
		application.NuevoAuditUseCase(relojMock{}, logger),
		nil,
	)

	if _, err := uc.Ejecutar(context.Background(), application.NotifyUserCommand{
		Title: "Aviso",
		Body:  "Sin salida",
	}); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}
}

func TestNotifyUser_EntradaInvalida(t *testing.T) {
	t.Parallel()

	uc := application.NuevoNotifyUserUseCase(&desktopNotificationMock{}, nil, nil, nil)
	if _, err := uc.Ejecutar(context.Background(), application.NotifyUserCommand{
		Title: "",
		Body:  "cuerpo",
	}); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}
