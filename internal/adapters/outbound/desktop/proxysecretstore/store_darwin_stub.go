// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin && !cgo

package proxysecretstore

import (
	"context"
	"runtime"

	"grxfirma/internal/ports"
)

func New() *Store { return &Store{} }

type Store struct{}

func (s *Store) Store(context.Context, string, ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	return ports.ProxySecretDescriptor{}, ErrNoDisponibleEnEstaPlataforma
}

func (s *Store) Load(context.Context, string) (ports.ProxySecretMaterial, error) {
	return ports.ProxySecretMaterial{}, ErrNoDisponibleEnEstaPlataforma
}

func (s *Store) Delete(context.Context, string) error {
	return ErrNoDisponibleEnEstaPlataforma
}

func (s *Store) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	return ports.ProxySecretStoreStatus{
		Available: false,
		Platform:  runtime.GOOS,
		Backend:   "keychain",
		Reason:    "proxysecretstore: backend Keychain requiere build con cgo en macOS",
	}, nil
}

var _ ports.ProxySecretStore = (*Store)(nil)
