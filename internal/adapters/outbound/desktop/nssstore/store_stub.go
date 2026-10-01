// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux

// Package nssstore stub para plataformas distintas de Linux.
// NSS (certutil) solo está disponible en Linux; en otros sistemas operativos
// los certificados de Firefox/Chrome se acceden vía los adaptadores nativos
// (wincertstore, macoskeychain).
package nssstore

import (
	"context"
	"errors"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ErrNoDisponibleEnEstaPlataforma indica que NSS no está soportado fuera de Linux.
var ErrNoDisponibleEnEstaPlataforma = errors.New("nssstore: solo disponible en Linux")

// Almacen es el stub del almacén NSS para plataformas no-Linux.
type Almacen struct{}

// New retorna un Almacen stub que siempre falla.
func New() *Almacen { return &Almacen{} }

// NewConRutas retorna un Almacen stub que siempre falla.
func NewConRutas(_ string, _ []string) *Almacen { return &Almacen{} }

// NewConHerramientas retorna un Almacen stub que siempre falla.
func NewConHerramientas(_, _ string, _ []string) *Almacen { return &Almacen{} }

// List retorna siempre ErrNoDisponibleEnEstaPlataforma.
func (a *Almacen) List(_ context.Context) ([]domain.CertificateRef, error) {
	return nil, ErrNoDisponibleEnEstaPlataforma
}

// KeyFor retorna siempre ErrNoDisponibleEnEstaPlataforma.
func (a *Almacen) KeyFor(_ context.Context, _ domain.CertificateRef) (ports.SigningKey, error) {
	return nil, ErrNoDisponibleEnEstaPlataforma
}
