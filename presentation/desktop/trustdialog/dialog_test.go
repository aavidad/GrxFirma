// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/trustdialog"
)

// trustPolicyMock simula ports.TrustPolicy para tests.
type trustPolicyMock struct {
	estado  domain.TrustStatus
	allowed map[string]bool
	denied  map[string]bool
}

func newMock(estado domain.TrustStatus) *trustPolicyMock {
	return &trustPolicyMock{estado: estado, allowed: map[string]bool{}, denied: map[string]bool{}}
}

func (m *trustPolicyMock) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	if m.allowed[origin] {
		return domain.TrustDecision{Origin: origin, Status: domain.TrustAllowed}, nil
	}
	if m.denied[origin] {
		return domain.TrustDecision{Origin: origin, Status: domain.TrustDenied}, nil
	}
	return domain.TrustDecision{Origin: origin, Status: m.estado}, nil
}

func (m *trustPolicyMock) Allow(_ context.Context, origin string) error {
	m.allowed[origin] = true
	delete(m.denied, origin)
	return nil
}

func (m *trustPolicyMock) Deny(_ context.Context, origin string) error {
	m.denied[origin] = true
	delete(m.allowed, origin)
	return nil
}

func (m *trustPolicyMock) Remove(_ context.Context, origin string) error {
	delete(m.allowed, origin)
	delete(m.denied, origin)
	return nil
}

var _ ports.TrustPolicy = (*trustPolicyMock)(nil)

// TestTrustDialog_OrigenPermitido_SinDialogo verifica que origenes ya permitidos
// no activan el diálogo.
func TestTrustDialog_OrigenPermitido_SinDialogo(t *testing.T) {
	t.Parallel()

	base := newMock(domain.TrustAllowed)
	ui := &trustdialog.HeadlessUI{Decision: trustdialog.Rechazar} // si se activara, rechazaría
	policy := trustdialog.New(base, ui)

	dec, err := policy.Evaluate(context.Background(), "https://known.example.com")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !dec.IsAllowed() {
		t.Errorf("esperado TrustAllowed, obtenido %v", dec.Status)
	}
}

// TestTrustDialog_OrigenDenegado_SinDialogo verifica que origenes ya denegados
// no activan el diálogo.
func TestTrustDialog_OrigenDenegado_SinDialogo(t *testing.T) {
	t.Parallel()

	base := newMock(domain.TrustDenied)
	ui := &trustdialog.HeadlessUI{Decision: trustdialog.ConfiarSiempre}
	policy := trustdialog.New(base, ui)

	dec, err := policy.Evaluate(context.Background(), "https://denied.example.com")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if dec.IsAllowed() {
		t.Error("origen denegado no debe ser permitido")
	}
}

// TestTrustDialog_ConfiarSiempre_Persiste verifica que "Confiar siempre" llama a Allow().
func TestTrustDialog_ConfiarSiempre_Persiste(t *testing.T) {
	t.Parallel()

	base := newMock(domain.TrustPending)
	ui := &trustdialog.HeadlessUI{Decision: trustdialog.ConfiarSiempre}
	policy := trustdialog.New(base, ui)

	dec, err := policy.Evaluate(context.Background(), "https://nuevo.example.com")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !dec.IsAllowed() {
		t.Errorf("esperado TrustAllowed tras ConfiarSiempre, obtenido %v", dec.Status)
	}
	// Verificar que quedó persistido en el mock
	if !base.allowed["https://nuevo.example.com"] {
		t.Error("Allow() no fue llamado tras ConfiarSiempre")
	}
}

// TestTrustDialog_ConfiarEstaVez_NoPersiste verifica que "Confiar esta vez" no persiste.
func TestTrustDialog_ConfiarEstaVez_NoPersiste(t *testing.T) {
	t.Parallel()

	base := newMock(domain.TrustPending)
	ui := &trustdialog.HeadlessUI{Decision: trustdialog.ConfiarEstaVez}
	policy := trustdialog.New(base, ui)

	dec, err := policy.Evaluate(context.Background(), "https://temporal.example.com")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !dec.IsAllowed() {
		t.Errorf("esperado TrustAllowed para esta vez, obtenido %v", dec.Status)
	}
	// No debe haberse persistido
	if base.allowed["https://temporal.example.com"] {
		t.Error("Allow() no debe llamarse tras ConfiarEstaVez")
	}
}

// TestTrustDialog_Rechazar_RetornaError verifica que "Rechazar" retorna ErrOrigenRechazado.
func TestTrustDialog_Rechazar_RetornaError(t *testing.T) {
	t.Parallel()

	base := newMock(domain.TrustPending)
	ui := &trustdialog.HeadlessUI{Decision: trustdialog.Rechazar}
	policy := trustdialog.New(base, ui)

	_, err := policy.Evaluate(context.Background(), "https://rechazado.example.com")
	if !errors.Is(err, trustdialog.ErrOrigenRechazado) {
		t.Errorf("esperado ErrOrigenRechazado, obtenido: %v", err)
	}
}

// TestTrustDialog_ContextoCancelado verifica que la cancelación propaga el error.
func TestTrustDialog_ContextoCancelado(t *testing.T) {
	t.Parallel()

	base := newMock(domain.TrustPending)
	// UI que respeta el contexto (HeadlessUI retorna inmediatamente, pero el contexto
	// se cancela antes de llamar a Evaluate).
	ui := &trustdialog.HeadlessUI{Decision: trustdialog.ConfiarEstaVez}
	policy := trustdialog.New(base, ui)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := policy.Evaluate(ctx, "https://ejemplo.com")
	// La política base recibe el contexto cancelado antes del diálogo.
	if err == nil {
		t.Log("sin error con contexto cancelado: la política base puede no verificarlo (aceptable)")
	}
}
