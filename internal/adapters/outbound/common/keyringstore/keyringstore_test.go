// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package keyringstore

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

// Los tests usan el backend simulado de go-keyring: no requieren un demonio
// Secret Service y no tocan el almacén real del desarrollador.
func TestMain(m *testing.M) {
	keyring.MockInit()
	m.Run()
}

func TestAlmacenCicloCompleto(t *testing.T) {
	a := NewConServicio("GrxFirma-test")
	ctx := context.Background()
	secreto := []byte{0x00, 0x01, 0xFF, 0x7F, 0x80} // binario arbitrario

	if err := a.Store(ctx, "clave-p12", secreto); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	leido, err := a.Load(ctx, "clave-p12")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !bytes.Equal(leido, secreto) {
		t.Errorf("Load() = %x, quiere %x", leido, secreto)
	}

	// Sobrescritura.
	if err := a.Store(ctx, "clave-p12", []byte("nuevo")); err != nil {
		t.Fatalf("Store() sobrescribiendo error = %v", err)
	}
	leido, err = a.Load(ctx, "clave-p12")
	if err != nil || string(leido) != "nuevo" {
		t.Errorf("tras sobrescribir: %q, %v", leido, err)
	}

	if err := a.Delete(ctx, "clave-p12"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := a.Load(ctx, "clave-p12"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("Load tras Delete debe dar ErrNoEncontrado, dio %v", err)
	}
}

func TestAlmacenCasosBorde(t *testing.T) {
	a := NewConServicio("GrxFirma-test")
	ctx := context.Background()

	if err := a.Store(ctx, "", []byte("x")); err == nil {
		t.Error("Store con clave vacía debe fallar")
	}
	if _, err := a.Load(ctx, "no-existe"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("Load de clave inexistente debe dar ErrNoEncontrado, dio %v", err)
	}
	if err := a.Delete(ctx, "no-existe"); err != nil {
		t.Errorf("Delete de clave inexistente no debe fallar: %v", err)
	}

	cancelado, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.Store(cancelado, "k", []byte("v")); !errors.Is(err, context.Canceled) {
		t.Errorf("Store con contexto cancelado debe respetar ctx: %v", err)
	}
}
