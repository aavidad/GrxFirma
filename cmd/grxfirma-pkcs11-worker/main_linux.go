// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && cgo

// The worker is an internal pipe endpoint, not a shell signing utility. Native
// modules must never be loaded by a desktop/bootstrap process.
package main

import (
	"context"
	"errors"
	"os"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11store"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/signingpolicy"
)

const operationTimeout = 90 * time.Second

func main() { os.Exit(run()) }

func run() int {
	// No module, identity or PIN is accepted through arguments/environment.
	if len(os.Args) != 1 {
		return 64
	}
	if err := pkcs11worker.HardenProcess(); err != nil {
		return 70
	}
	output, err := pkcs11worker.PrivateProtocolOutput()
	if err != nil {
		return 70
	}
	defer output.Close()
	// Apply syscall restrictions to every Go/CGo thread before any dlopen
	// constructor can run. The parent supplies the filesystem/network boundary.
	if err := pkcs11worker.RestrictDriverSyscalls(); err != nil {
		return 70
	}
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		completed <- (pkcs11worker.Server{Factory: newBackend, ErrorCode: classifyError}).Serve(ctx, os.Stdin, output)
	}()
	select {
	case err := <-completed:
		if err != nil {
			return 74
		}
		return 0
	case <-ctx.Done():
		// Returning to os.Exit terminates even a blocked C call or Finalize.
		return 75
	}
}

type nativeBackend struct{ *pkcs11store.Almacen }

func (b *nativeBackend) Close() { b.Cerrar() }

func (b *nativeBackend) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := b.CheckAvailable(ctx); err != nil {
		return nil, err
	}
	return b.Almacen.List(ctx)
}

func newBackend(path string, source pkcs11worker.PINSource) (pkcs11worker.Backend, error) {
	resolved, err := pkcs11worker.ValidateIsolatedModulePath(path)
	if err != nil {
		return nil, pkcs11store.ErrModuloNoDisponible
	}
	provider := pkcs11store.PINProviderFunc(func(ctx context.Context, request pkcs11store.PINRequest) ([]byte, error) {
		if source == nil {
			return nil, pkcs11store.ErrPINUnavailable
		}
		return source(ctx, pkcs11worker.PINMode{
			ProtectedAuthenticationPath: request.ProtectedAuthenticationPath,
			ContextSpecific:             request.ContextSpecific,
		})
	})
	return &nativeBackend{pkcs11store.NewWithOptions(resolved, pkcs11store.Options{PINProvider: provider})}, nil
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, pkcs11store.ErrPINIncorrect):
		return "pin_incorrect"
	case errors.Is(err, pkcs11store.ErrPINLocked):
		return "pin_locked"
	case errors.Is(err, pkcs11store.ErrPINExpired):
		return "pin_expired"
	case errors.Is(err, pkcs11store.ErrPINUnavailable):
		return "pin_cancelled"
	case errors.Is(err, pkcs11store.ErrTokenUnavailable):
		return "device_removed"
	case errors.Is(err, pkcs11store.ErrModuloNoDisponible):
		return "driver_unavailable"
	case errors.Is(err, pkcs11store.ErrMechanismUnsupported):
		return "unsupported_algorithm"
	case errors.Is(err, pkcs11store.ErrIdentityNotFound), errors.Is(err, pkcs11store.ErrAmbiguousIdentity):
		return "key_unavailable"
	case errors.Is(err, signingpolicy.ErrCertificateUnsuitable):
		return "unsuitable_certificate"
	default:
		return "driver_failed"
	}
}
