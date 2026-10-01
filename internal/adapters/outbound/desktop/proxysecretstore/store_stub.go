// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !windows && !darwin

package proxysecretstore

import (
	"context"
	"runtime"

	"grxfirma/internal/ports"
)

type Store struct{}

func New() *Store { return &Store{} }

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
	backend, reason := backendReadinessForPlatform(runtime.GOOS)
	return ports.ProxySecretStoreStatus{
		Available: false,
		Platform:  runtime.GOOS,
		Backend:   backend,
		Reason:    reason,
	}, nil
}

var _ ports.ProxySecretStore = (*Store)(nil)
