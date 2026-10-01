// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !darwin || !cgo

// Package macoskeychain implementa ports.CertificateCatalog y ports.SigningKeyProvider
// usando CGo bridge a Security.framework de macOS.
// En plataformas distintas a macOS, este stub compila sin CGo y devuelve errores controlados.
package macoskeychain

import (
	"context"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Almacen implementa ports.CertificateCatalog y ports.SigningKeyProvider.
// En plataformas no-macOS no tiene acceso al Keychain.
type Almacen struct{}

// New crea un Almacen (stub sin funcionalidad en plataformas no-macOS).
func New() *Almacen {
	return &Almacen{}
}

// List retorna una lista vacía en plataformas no-macOS.
func (a *Almacen) List(_ context.Context) ([]domain.CertificateRef, error) {
	return nil, nil
}

// KeyFor retorna ErrNoDisponibleEnEstaPlataforma en plataformas no-macOS.
func (a *Almacen) KeyFor(_ context.Context, _ domain.CertificateRef) (ports.SigningKey, error) {
	return nil, ErrNoDisponibleEnEstaPlataforma
}

var _ ports.CertificateCatalog = (*Almacen)(nil)
var _ ports.SigningKeyProvider = (*Almacen)(nil)
