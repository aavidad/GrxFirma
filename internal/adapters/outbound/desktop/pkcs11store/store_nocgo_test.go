// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !cgo

package pkcs11store_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11store"
	"grxfirma/internal/ports"
)

func TestSinCGO_CatalogosVacios(t *testing.T) {
	t.Parallel()

	for nombre, almacen := range map[string]*pkcs11store.Almacen{
		"ruta explícita": pkcs11store.New("/opt/token/pkcs11.so"),
		"autodetección":  pkcs11store.NewAutodetect(),
	} {
		t.Run(nombre, func(t *testing.T) {
			refs, err := almacen.List(context.Background())
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(refs) != 0 {
				t.Fatalf("List() devolvió %d certificados; se esperaba catálogo vacío", len(refs))
			}

			tokens, err := almacen.Enumerate(context.Background())
			if err != nil {
				t.Fatalf("Enumerate() error = %v", err)
			}
			if len(tokens) != 0 {
				t.Fatalf("Enumerate() devolvió %d tokens; se esperaba catálogo vacío", len(tokens))
			}
		})
	}
}

func TestSinCGO_SignRechazaSinSimularFirma(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/opt/token/pkcs11.so")
	firma, err := almacen.Sign(context.Background(), ports.TokenRef{
		ID:    "pkcs11:slot:0",
		Label: "token de prueba",
	}, []byte("digest"))

	if firma != nil {
		t.Fatalf("Sign() devolvió una firma sin CGo: %x", firma)
	}
	if !errors.Is(err, pkcs11store.ErrCGONoDisponible) {
		t.Fatalf("Sign() error = %v; se esperaba ErrCGONoDisponible", err)
	}
	if !errors.Is(err, pkcs11store.ErrModuloNoDisponible) {
		t.Fatalf("Sign() error = %v; debe preservar ErrModuloNoDisponible", err)
	}
}

func TestSinCGO_RespetaContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	almacen := pkcs11store.NewAutodetect()

	if _, err := almacen.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List() error = %v; se esperaba context.Canceled", err)
	}
	if _, err := almacen.Enumerate(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Enumerate() error = %v; se esperaba context.Canceled", err)
	}
	if firma, err := almacen.Sign(ctx, ports.TokenRef{}, nil); firma != nil || !errors.Is(err, context.Canceled) {
		t.Errorf("Sign() = (%x, %v); se esperaba nil, context.Canceled", firma, err)
	}
}
