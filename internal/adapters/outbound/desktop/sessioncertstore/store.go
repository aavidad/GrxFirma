// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package sessioncertstore mantiene credenciales externas solo en memoria.
package sessioncertstore

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/secmem"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const maxCredentialBytes = 16 * 1024 * 1024

// Store es un overlay de catálogo y claves cuya vida termina con el proceso.
// No instala identidades ni conserva credenciales en disco. El importador de
// compatibilidad OpenSSL puede usar temporales privados que elimina al terminar.
type Store struct {
	mu    sync.RWMutex
	refs  map[string]domain.CertificateRef
	keys  map[string]ports.SigningKey
	order []string
}

func New() *Store {
	return &Store{
		refs: make(map[string]domain.CertificateRef),
		keys: make(map[string]ports.SigningKey),
	}
}

// Load admite PKCS#12/PFX y bundles PEM que contengan certificado y clave.
// Se copia y zeroiza el material de entrada usado durante el parseo.
func (s *Store) Load(ctx context.Context, data []byte, password string) (domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.CertificateRef{}, err
	}
	if s == nil {
		return domain.CertificateRef{}, errors.New("almacén temporal no configurado")
	}
	if len(data) == 0 {
		return domain.CertificateRef{}, errors.New("la credencial temporal no puede estar vacía")
	}
	if len(data) > maxCredentialBytes {
		return domain.CertificateRef{}, fmt.Errorf("la credencial temporal supera el tamaño máximo de %d MiB", maxCredentialBytes/(1024*1024))
	}

	working := append([]byte(nil), data...)
	defer secmem.Zeroize(working)
	identity, err := pkcs12importer.New().ImportIdentity(ctx, working, password)
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("no se pudo abrir la credencial temporal: %w", err)
	}
	if err := identity.Reference.Validate(); err != nil {
		return domain.CertificateRef{}, fmt.Errorf("credencial temporal inválida: %w", err)
	}

	key := desktopsigner.NuevaClaveLocalConCadena(identity.Signer, identity.Certificate, identity.Chain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refs == nil {
		s.refs = make(map[string]domain.CertificateRef)
	}
	if s.keys == nil {
		s.keys = make(map[string]ports.SigningKey)
	}
	if _, exists := s.refs[identity.Reference.ID]; !exists {
		s.order = append(s.order, identity.Reference.ID)
	}
	s.refs[identity.Reference.ID] = identity.Reference
	s.keys[identity.Reference.ID] = key
	return identity.Reference, nil
}

func (s *Store) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.CertificateRef, 0, len(s.order))
	for _, id := range s.order {
		if ref, ok := s.refs[id]; ok {
			result = append(result, ref)
		}
	}
	return result, nil
}

func (s *Store) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("almacén temporal no configurado")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.keys[certificate.ID]
	if !ok || key == nil {
		return nil, fmt.Errorf("credencial temporal no encontrada: %s", certificate.ID)
	}
	return key, nil
}

func (s *Store) Remove(certificateID string) {
	if s == nil || certificateID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.refs, certificateID)
	delete(s.keys, certificateID)
	for i, id := range s.order {
		if id == certificateID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.refs)
	clear(s.keys)
	s.order = nil
}

var _ ports.TemporaryCertificateStore = (*Store)(nil)
