// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type catalogoMetadataSpy struct {
	refs     []domain.CertificateRef
	listErr  error
	keyErr   error
	keyCalls int
}

func (s *catalogoMetadataSpy) List(context.Context) ([]domain.CertificateRef, error) {
	return s.refs, s.listErr
}

func (s *catalogoMetadataSpy) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	s.keyCalls++
	return nil, s.keyErr
}

func TestCatalogoFirmableUsaSoloMetadataSinAbrirClaves(t *testing.T) {
	refs := []domain.CertificateRef{
		{ID: "disponible", HasSigningKey: true},
		{ID: "bloqueada", SigningKeyNeedsUnlock: true},
		{ID: "solo-publica"},
		{ID: "desconocida"},
	}
	spy := &catalogoMetadataSpy{refs: refs, keyErr: errors.New("no debe solicitar PIN")}
	catalogo := &catalogoFirmable{base: spy}
	got, err := catalogo.List(context.Background())
	if err != nil || !reflect.DeepEqual(got, refs[:2]) {
		t.Fatalf("catálogo=%+v error=%v", got, err)
	}
	if spy.keyCalls != 0 {
		t.Fatal("listado accedió al proveedor de claves")
	}
	if got[1].HasSigningKey || !got[1].SigningKeyNeedsUnlock {
		t.Fatal("se anunció disponible una clave aún bloqueada")
	}
}

func TestCatalogoFirmablePropagaErrorSinAbrirClaves(t *testing.T) {
	want := errors.New("fallo catálogo sintético")
	spy := &catalogoMetadataSpy{listErr: want}
	got, err := (&catalogoFirmable{base: spy}).List(context.Background())
	if got != nil || !errors.Is(err, want) || spy.keyCalls != 0 {
		t.Fatalf("error/listado alterado: %v", err)
	}
}

func TestProveedorTokenNoAplicablePreservaCausaAnterior(t *testing.T) {
	want := errors.New("causa original del proveedor seleccionado")
	for _, ignored := range []error{tokenruntime.ErrNotApplicable, tokenruntime.ErrIdentityUnknown} {
		t.Run(ignored.Error(), func(t *testing.T) {
			prior := &catalogoMetadataSpy{keyErr: want}
			tokens := &catalogoMetadataSpy{keyErr: fmt.Errorf("contexto fijo: %w", ignored)}
			provider := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{prior, tokens}}
			_, err := provider.KeyFor(context.Background(), domain.CertificateRef{ID: "otro-almacen"})
			if !errors.Is(err, want) {
				t.Fatalf("token no aplicable ocultó error original: %v", err)
			}
		})
	}
}

func TestProveedorTokenConFalloRealNoSeIgnora(t *testing.T) {
	want := tokenruntime.ErrHelperUnavailable
	provider := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{&catalogoMetadataSpy{keyErr: want}}}
	_, err := provider.KeyFor(context.Background(), domain.CertificateRef{ID: "token"})
	if !errors.Is(err, want) {
		t.Fatalf("fallo real de token ocultado: %v", err)
	}
}
