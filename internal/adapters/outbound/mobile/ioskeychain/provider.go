// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ioskeychain

import (
	"context"
	"errors"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Resolver abstrae la busqueda de claves en iOS Keychain / Secure Enclave.
type Resolver interface {
	ResolveKey(ctx context.Context, certificate domain.CertificateRef) (ResolvedKey, error)
}

// ResolveFunc adapta una funcion al contrato Resolver.
type ResolveFunc func(ctx context.Context, certificate domain.CertificateRef) (ResolvedKey, error)

// ResolveKey ejecuta la funcion adaptada.
func (f ResolveFunc) ResolveKey(ctx context.Context, certificate domain.CertificateRef) (ResolvedKey, error) {
	return f(ctx, certificate)
}

// ResolvedKey representa la referencia minima devuelta por la capa nativa.
type ResolvedKey struct {
	KeyID         string
	KeychainTag   string
	SecureEnclave bool
}

// KeyRef implementa ports.SigningKey para referencias opacas a iOS Keychain.
type KeyRef struct {
	id            string
	keychainTag   string
	secureEnclave bool
}

// KeyID devuelve el identificador estable de la clave.
func (k *KeyRef) KeyID() string { return k.id }

// KeychainTag expone la etiqueta nativa resuelta por la capa iOS.
func (k *KeyRef) KeychainTag() string { return k.keychainTag }

// UsesSecureEnclave indica si la referencia apunta a material protegido por Secure Enclave.
func (k *KeyRef) UsesSecureEnclave() bool { return k.secureEnclave }

// CertificateChainDER devuelve nil por ahora en mobile Keychain hasta que el
// bridge nativo proporcione la cadena completa.
func (k *KeyRef) CertificateChainDER() [][]byte {
	return nil
}

// Provider implementa SigningKeyProvider sobre iOS Keychain / Secure Enclave.
type Provider struct {
	resolver Resolver
}

// Nuevo crea un Provider asociado a un resolver nativo.
func Nuevo(resolver Resolver) *Provider {
	return &Provider{resolver: resolver}
}

// KeyFor devuelve la clave asociada al certificado pedido.
func (p Provider) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.resolver == nil {
		return nil, errors.New("ios-keychain: bridge nativo no configurado")
	}
	resolved, err := p.resolver.ResolveKey(ctx, certificate)
	if err != nil {
		return nil, err
	}
	keyID := strings.TrimSpace(resolved.KeyID)
	if keyID == "" {
		keyID = strings.TrimSpace(certificate.ID)
	}
	if keyID == "" {
		keyID = strings.TrimSpace(certificate.Fingerprint)
	}
	if keyID == "" {
		return nil, errors.New("ios-keychain: el bridge nativo no devolvio un identificador de clave")
	}
	return &KeyRef{
		id:            keyID,
		keychainTag:   strings.TrimSpace(resolved.KeychainTag),
		secureEnclave: resolved.SecureEnclave,
	}, nil
}

var _ ports.SigningKeyProvider = (*Provider)(nil)
var _ ports.SigningKey = (*KeyRef)(nil)
