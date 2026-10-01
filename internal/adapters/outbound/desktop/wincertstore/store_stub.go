// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

// Package wincertstore implementa los contratos de catálogo y clave sobre el
// almacén personal "MY" de Windows CryptoAPI.
// En plataformas distintas a Windows, este stub compila sin dependencias nativas
// y devuelve una lista vacía.
package wincertstore

import (
	"context"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Almacen implementa ports.CertificateCatalog. En plataformas no-Windows no tiene
// acceso al CertStore y siempre devuelve una lista vacía.
type Almacen struct{}

// New crea un Almacen (stub sin funcionalidad en plataformas no-Windows).
func New() *Almacen {
	return &Almacen{}
}

// List retorna una lista vacía en plataformas no-Windows.
func (a *Almacen) List(_ context.Context) ([]domain.CertificateRef, error) {
	return nil, nil
}

// KeyFor informa de forma explícita que Windows CertStore no está disponible.
func (a *Almacen) KeyFor(_ context.Context, _ domain.CertificateRef) (ports.SigningKey, error) {
	return nil, ErrNoDisponibleEnEstaPlataforma
}

var _ ports.CertificateCatalog = (*Almacen)(nil)
var _ ports.SigningKeyProvider = (*Almacen)(nil)
