// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package tokenruntime composes explicitly configured local token helpers.
// Drivers and PINs are never selected through browser options or environment.
package tokenruntime

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	ErrNotApplicable      = errors.New("acceso a tokens no habilitado en esta compilación")
	ErrInvalidConfig      = errors.New("configuración local de tokens no válida")
	ErrHelperUnavailable  = errors.New("auxiliar local de tokens no disponible")
	ErrCatalogUnavailable = errors.New("catálogo de tokens no disponible")
	ErrIdentityUnknown    = errors.New("identidad no presente en el catálogo local de tokens")
)

type Prompt func(context.Context, domain.CertificateRef, pkcs11worker.PINMode) ([]byte, error)

type source interface {
	ports.CertificateCatalog
	ports.SigningKeyProvider
}
type sourceFactory func(module string, pin pkcs11worker.PINSource) source
type route struct {
	module    string
	reference domain.CertificateRef
}

// Runtime does not retain native sessions or PIN buffers. routes contains only
// identities last observed by List, preventing KeyFor from probing other keys.
type Runtime struct {
	mu         sync.Mutex
	modules    []string
	factory    sourceFactory
	prompt     Prompt
	routes     map[string]route
	inactive   error
	diagnostic error
}

var _ ports.CertificateCatalog = (*Runtime)(nil)
var _ ports.SigningKeyProvider = (*Runtime)(nil)

// New reads configuration only in an explicitly tagged Linux preview build.
// Default and production constructors do not access configDir or start helpers.
func New(configDir string, prompt func(context.Context, domain.CertificateRef, pkcs11worker.PINMode) ([]byte, error)) *Runtime {
	return newRuntime(configDir, Prompt(prompt))
}

// Diagnostico returns a sanitized latest diagnostic, never module paths,
// certificate details, a PIN, or arbitrary driver/process error strings.
func (r *Runtime) Diagnostico() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.diagnostic
}

func (r *Runtime) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.inactive != nil {
		if errors.Is(r.inactive, ErrNotApplicable) {
			return nil, nil
		}
		return nil, r.inactive
	}
	if len(r.modules) == 0 {
		return nil, nil
	}
	var refs []domain.CertificateRef
	var diagnostics []error
	routes := make(map[string]route)
	ambiguous := make(map[string]bool)
	successful := 0
	for index, module := range r.modules {
		// Listing has no PIN callback, even if a future transport regresses.
		entries, err := r.factory(module, nil).List(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("módulo local %d: %w", index+1, ErrCatalogUnavailable))
			continue
		}
		successful++
		for _, ref := range entries {
			if !validReference(ref) || (ref.HasSigningKey && ref.SigningKeyNeedsUnlock) {
				diagnostics = append(diagnostics, fmt.Errorf("módulo local %d: %w", index+1, ErrCatalogUnavailable))
				continue
			}
			if _, exists := routes[ref.ID]; exists || ambiguous[ref.ID] {
				delete(routes, ref.ID)
				ambiguous[ref.ID] = true
				diagnostics = append(diagnostics, fmt.Errorf("módulo local %d: %w", index+1, ErrIdentityUnknown))
				continue
			}
			routes[ref.ID] = route{module: module, reference: ref}
			refs = append(refs, ref)
		}
	}
	// Remove both sides of an identity collision instead of guessing a module.
	filtered := refs[:0]
	for _, ref := range refs {
		if !ambiguous[ref.ID] {
			filtered = append(filtered, ref)
		}
	}
	r.mu.Lock()
	r.routes = routes
	r.diagnostic = errors.Join(diagnostics...)
	r.mu.Unlock()
	if successful == 0 {
		return nil, ErrCatalogUnavailable
	}
	// The aggregate catalog discards sources returning errors; preserve good
	// modules while making partial failure visible through Diagnostico.
	return filtered, nil
}

func (r *Runtime) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.inactive != nil {
		return nil, r.inactive
	}
	if !validReference(ref) {
		return nil, ErrIdentityUnknown
	}
	r.mu.Lock()
	selected, ok := r.routes[ref.ID]
	r.mu.Unlock()
	if !ok || !strings.EqualFold(selected.reference.Fingerprint, ref.Fingerprint) {
		return nil, ErrIdentityUnknown
	}
	// Use the catalog's original ref, never a subject or label submitted by a
	// web client. The PIN source is bound to exactly this module and identity.
	pin := func(ctx context.Context, mode pkcs11worker.PINMode) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if r.prompt == nil {
			return nil, pkcs11worker.ErrPINCancelled
		}
		return r.prompt(ctx, selected.reference, mode)
	}
	return r.factory(selected.module, pin).KeyFor(ctx, selected.reference)
}

func validReference(ref domain.CertificateRef) bool {
	if len(ref.ID) == 0 || len(ref.ID) > 2048 || strings.ContainsRune(ref.ID, 0) || len(ref.Fingerprint) != 64 {
		return false
	}
	_, err := hex.DecodeString(ref.Fingerprint)
	return err == nil
}
