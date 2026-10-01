// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/certpicker"
)

func makeCert(id, subject, issuer string) domain.CertificateRef {
	return domain.CertificateRef{
		ID:          id,
		Subject:     subject,
		Issuer:      issuer,
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		Fingerprint: id + "-fingerprint",
	}
}

func TestHeadlessSelector_ListaVacia(t *testing.T) {
	sel := certpicker.NewHeadless()
	_, err := sel.Select(context.Background(), nil)
	if err == nil {
		t.Fatal("se esperaba error para lista vacía")
	}
}

func TestHeadlessSelector_UnCertificado(t *testing.T) {
	sel := certpicker.NewHeadless()
	cert := makeCert("cert-1", "CN=Usuario", "CN=AC Raiz")
	got, err := sel.Select(context.Background(), []domain.CertificateRef{cert})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got.Certificado.ID != cert.ID {
		t.Errorf("esperado ID=%q, obtenido ID=%q", cert.ID, got.Certificado.ID)
	}
	if got.Recuerdo != certpicker.NoRecordar {
		t.Errorf("se esperaba modo de recuerdo vacío por defecto, obtenido %q", got.Recuerdo)
	}
}

func TestHeadlessSelector_VariosCertificados_SeleccionaPrimero(t *testing.T) {
	sel := certpicker.NewHeadless()
	certs := []domain.CertificateRef{
		makeCert("cert-1", "CN=Primero", "CN=AC"),
		makeCert("cert-2", "CN=Segundo", "CN=AC"),
		makeCert("cert-3", "CN=Tercero", "CN=AC"),
	}
	got, err := sel.Select(context.Background(), certs)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got.Certificado.ID != "cert-1" {
		t.Errorf("esperado el primer certificado (cert-1), obtenido %q", got.Certificado.ID)
	}
}

func TestHeadlessSelector_ContextoCancelado(t *testing.T) {
	sel := certpicker.NewHeadless()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar antes de llamar

	certs := []domain.CertificateRef{
		makeCert("cert-1", "CN=Usuario", "CN=AC"),
	}
	_, err := sel.Select(ctx, certs)
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("se esperaba context.Canceled, obtenido: %v", err)
	}
}

func TestErrSeleccionCancelada_Distinguible(t *testing.T) {
	// ErrSeleccionCancelada debe ser distinguible de context.Canceled y otros errores.
	err := certpicker.ErrSeleccionCancelada

	if errors.Is(err, context.Canceled) {
		t.Error("ErrSeleccionCancelada no debe ser context.Canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Error("ErrSeleccionCancelada no debe ser context.DeadlineExceeded")
	}
	if !errors.Is(err, certpicker.ErrSeleccionCancelada) {
		t.Error("ErrSeleccionCancelada debe ser identificable con errors.Is")
	}
}

func TestCertSelector_InterfazCumplida(t *testing.T) {
	// Verificación en tiempo de compilación: HeadlessSelector implementa CertSelector.
	var _ certpicker.CertSelector = certpicker.NewHeadless()
}
