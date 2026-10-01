// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type signingKeyProviderFunc func(context.Context, domain.CertificateRef) (ports.SigningKey, error)

func (f signingKeyProviderFunc) KeyFor(
	ctx context.Context,
	ref domain.CertificateRef,
) (ports.SigningKey, error) {
	return f(ctx, ref)
}

type closableSigningKey struct {
	id     string
	closed int
}

func (k *closableSigningKey) KeyID() string {
	return k.id
}

func (*closableSigningKey) CertificateChainDER() [][]byte {
	return nil
}

func (k *closableSigningKey) Close() {
	k.closed++
}

func TestConstruirFuentesCertificados_IncluyeProveedorWindows(t *testing.T) {
	_, provider, err := construirFuentesCertificados(
		context.Background(),
		"",
		"",
		"",
		"",
		t.TempDir(),
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("construirFuentesCertificados() error = %v", err)
	}

	aggregate, ok := provider.(*proveedorClavesAgregado)
	if !ok {
		t.Fatalf("proveedor = %T, want *proveedorClavesAgregado", provider)
	}
	found := 0
	for _, source := range aggregate.fuentes {
		if _, ok := source.(*wincertstore.Almacen); ok {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("proveedores Windows = %d, want 1", found)
	}
}

func TestProveedorClavesAgregado_NoOcultaCausaConProveedorNoAplicablePosterior(t *testing.T) {
	t.Parallel()

	causa := errors.New("fallo real adquiriendo la clave")
	proveedorConCausa := signingKeyProviderFunc(func(
		context.Context,
		domain.CertificateRef,
	) (ports.SigningKey, error) {
		return nil, causa
	})
	noAplicables := []error{
		fmt.Errorf("keychain ausente: %w", macoskeychain.ErrNoDisponibleEnEstaPlataforma),
		fmt.Errorf("certstore ausente: %w", wincertstore.ErrNoDisponibleEnEstaPlataforma),
		errors.New("nssstore: solo disponible en Linux"),
	}

	for _, noAplicable := range noAplicables {
		noAplicable := noAplicable
		t.Run(noAplicable.Error(), func(t *testing.T) {
			t.Parallel()
			proveedorAusente := signingKeyProviderFunc(func(
				context.Context,
				domain.CertificateRef,
			) (ports.SigningKey, error) {
				return nil, noAplicable
			})
			agregado := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{
				proveedorConCausa,
				proveedorAusente,
			}}

			clave, err := agregado.KeyFor(context.Background(), domain.CertificateRef{ID: "test"})
			if clave != nil {
				t.Fatalf("KeyFor() clave = %T, want nil", clave)
			}
			if !errors.Is(err, causa) {
				t.Fatalf("KeyFor() error = %v, want causa real %v", err, causa)
			}
		})
	}
}

func TestProveedorClavesAgregado_CierraClaveErroneaAntesDelFallback(t *testing.T) {
	t.Parallel()

	descartada := &closableSigningKey{id: "descartada"}
	esperada := &closableSigningKey{id: "fallback"}
	proveedorErroneo := signingKeyProviderFunc(func(
		context.Context,
		domain.CertificateRef,
	) (ports.SigningKey, error) {
		return descartada, errors.New("clave parcial")
	})
	proveedorFallback := signingKeyProviderFunc(func(
		context.Context,
		domain.CertificateRef,
	) (ports.SigningKey, error) {
		if descartada.closed != 1 {
			return nil, fmt.Errorf(
				"clave descartada cerrada %d veces antes del fallback",
				descartada.closed,
			)
		}
		return esperada, nil
	})
	agregado := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{
		proveedorErroneo,
		proveedorFallback,
	}}

	clave, err := agregado.KeyFor(context.Background(), domain.CertificateRef{ID: "test"})
	if err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	if clave != esperada {
		t.Fatalf("KeyFor() clave = %T %p, want fallback %p", clave, clave, esperada)
	}
	if descartada.closed != 1 {
		t.Fatalf("clave descartada cerrada %d veces, want 1", descartada.closed)
	}
}
