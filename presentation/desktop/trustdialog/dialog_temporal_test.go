// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

type baseFija struct{ estado domain.TrustStatus }

func (b *baseFija) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	return domain.TrustDecision{Origin: origin, Status: b.estado}, nil
}
func (b *baseFija) Allow(context.Context, string) error  { return nil }
func (b *baseFija) Deny(context.Context, string) error   { return nil }
func (b *baseFija) Remove(context.Context, string) error { return nil }

type uiContador struct {
	llamadas atomic.Int32
	espera   chan struct{}
}

func (u *uiContador) PedirDecision(context.Context, string) (DecisionUnicaVez, error) {
	u.llamadas.Add(1)
	if u.espera != nil {
		<-u.espera
	}
	return ConfiarEstaVez, nil
}

// A-05: "Confiar esta vez" no vuelve a preguntar en cada validación de la
// misma operación, y caduca.
func TestConfiarEstaVezSeRecuerdaUnRato(t *testing.T) {
	ahora := time.Unix(1_000_000, 0)
	ui := &uiContador{}
	base := &baseFija{estado: domain.TrustPending}
	p := New(base, ui)
	p.now = func() time.Time { return ahora }

	for i := 0; i < 3; i++ {
		if d, err := p.Evaluate(context.Background(), "https://x.example"); err != nil || !d.IsAllowed() {
			t.Fatalf("Evaluate %d: %v %v", i, d, err)
		}
	}
	if n := ui.llamadas.Load(); n != 1 {
		t.Fatalf("PedirDecision llamado %d veces; se esperaba 1", n)
	}
	ahora = ahora.Add(DuracionConfiarEstaVez + time.Second)
	_, _ = p.Evaluate(context.Background(), "https://x.example")
	if n := ui.llamadas.Load(); n != 2 {
		t.Fatalf("tras caducar debía volver a preguntar: %d llamadas", n)
	}
	// Si la política base pasa a denegar, la memoria temporal no lo salta.
	base.estado = domain.TrustDenied
	if d, _ := p.Evaluate(context.Background(), "https://x.example"); d.IsAllowed() {
		t.Fatal("la política base manda sobre la confianza temporal")
	}
}

func TestDialogosSimultaneosDelMismoOrigenSeAgrupan(t *testing.T) {
	ui := &uiContador{espera: make(chan struct{})}
	p := New(&baseFija{estado: domain.TrustPending}, ui)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = p.Evaluate(context.Background(), "https://y.example")
		}()
	}
	for ui.llamadas.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(ui.espera)
	wg.Wait()
	if n := ui.llamadas.Load(); n != 1 {
		t.Fatalf("se mostraron %d diálogos; se esperaba 1", n)
	}
}
