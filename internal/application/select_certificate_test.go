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
	"grxfirma/internal/ports"
)

func TestSelectCertificate_FiltraPorSubjectEIssuer(t *testing.T) {
	uc := application.NuevoSelectCertificateUseCase(
		&catalogoSelectMock{certs: []domain.CertificateRef{
			certificadoSelect("1", "CN=Ana Lopez", "CN=FNMT", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			certificadoSelect("2", "CN=Pedro Ruiz", "CN=ACCV", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
		}},
		aprobadorSelectMock{respuesta: true},
		nil,
		relojSelectMock{now: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)},
	)

	resultado, err := uc.Ejecutar(context.Background(), application.SelectCertificateCommand{
		SubjectFilter:   "ana",
		IssuerFilter:    "fnmt",
		SoloNoCaducados: true,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if resultado.Selection.Certificate.ID != "1" {
		t.Fatalf("certificado inesperado: %s", resultado.Selection.Certificate.ID)
	}
}

func TestSelectCertificate_ExcluyeCaducados(t *testing.T) {
	uc := application.NuevoSelectCertificateUseCase(
		&catalogoSelectMock{certs: []domain.CertificateRef{
			certificadoSelect("1", "CN=Caducado", "CN=FNMT", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		}},
		aprobadorSelectMock{respuesta: true},
		nil,
		relojSelectMock{now: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)},
	)

	_, err := uc.Ejecutar(context.Background(), application.SelectCertificateCommand{
		SoloNoCaducados: true,
	})
	if err == nil {
		t.Fatal("se esperaba error al no quedar certificados vigentes")
	}
}

func TestSelectCertificate_VariosCandidatosPideAprobacionYPublicaEvento(t *testing.T) {
	contador := &publicadorSelectMock{}
	uc := application.NuevoSelectCertificateUseCase(
		&catalogoSelectMock{certs: []domain.CertificateRef{
			certificadoSelect("1", "CN=Ana", "CN=FNMT", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			certificadoSelect("2", "CN=Pedro", "CN=FNMT", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
		}},
		aprobadorSelectMock{respuesta: true},
		contador,
		relojSelectMock{now: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)},
	)

	resultado, err := uc.Ejecutar(context.Background(), application.SelectCertificateCommand{})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if resultado.Selection.Certificate.ID != "1" {
		t.Fatalf("se esperaba el primer certificado compatible, obtenido %s", resultado.Selection.Certificate.ID)
	}
	if len(contador.eventos) != 1 {
		t.Fatalf("se esperaba un evento, obtenidos %d", len(contador.eventos))
	}
}

func TestSelectCertificate_CancelacionUsuario(t *testing.T) {
	uc := application.NuevoSelectCertificateUseCase(
		&catalogoSelectMock{certs: []domain.CertificateRef{
			certificadoSelect("1", "CN=Ana", "CN=FNMT", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			certificadoSelect("2", "CN=Pedro", "CN=FNMT", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
		}},
		aprobadorSelectMock{respuesta: false},
		nil,
		relojSelectMock{now: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)},
	)

	_, err := uc.Ejecutar(context.Background(), application.SelectCertificateCommand{})
	if err == nil {
		t.Fatal("se esperaba error por cancelacion")
	}
}

func TestSelectCertificate_ErrorCatalogo(t *testing.T) {
	uc := application.NuevoSelectCertificateUseCase(
		&catalogoSelectMock{err: errors.New("catalogo fuera de servicio")},
		aprobadorSelectMock{respuesta: true},
		nil,
		relojSelectMock{now: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)},
	)

	_, err := uc.Ejecutar(context.Background(), application.SelectCertificateCommand{})
	if err == nil {
		t.Fatal("se esperaba error del catalogo")
	}
}

type catalogoSelectMock struct {
	certs []domain.CertificateRef
	err   error
}

func (c *catalogoSelectMock) List(context.Context) ([]domain.CertificateRef, error) {
	return c.certs, c.err
}

type aprobadorSelectMock struct {
	respuesta bool
	err       error
}

func (a aprobadorSelectMock) Request(context.Context, string) (bool, error) {
	return a.respuesta, a.err
}

type publicadorSelectMock struct {
	eventos []ports.Event
}

func (p *publicadorSelectMock) Publish(_ context.Context, event ports.Event) error {
	p.eventos = append(p.eventos, event)
	return nil
}

type relojSelectMock struct {
	now time.Time
}

func (r relojSelectMock) Now() time.Time {
	return r.now
}

func certificadoSelect(id, subject, issuer string, notAfter time.Time) domain.CertificateRef {
	return domain.CertificateRef{
		ID:          id,
		Subject:     subject,
		Issuer:      issuer,
		NotAfter:    notAfter,
		Fingerprint: "fp-" + id,
	}
}
