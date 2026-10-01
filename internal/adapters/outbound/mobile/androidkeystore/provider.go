// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package androidkeystore

import (
	"context"
	"errors"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Resolver abstrae la resolucion de aliases y referencias reales en Android Keystore.
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
	KeyID string
	Alias string
}

// KeyRef implementa ports.SigningKey para referencias opacas a Android Keystore.
type KeyRef struct {
	id    string
	alias string
}

// KeyID devuelve el identificador estable de la clave.
func (k *KeyRef) KeyID() string { return k.id }

// Alias expone el alias nativo resuelto para depuracion o bridges posteriores.
func (k *KeyRef) Alias() string { return k.alias }

// CertificateChainDER devuelve nil por ahora en mobile Keychain hasta que el
// bridge nativo proporcione la cadena completa.
func (k *KeyRef) CertificateChainDER() [][]byte {
	return nil
}

// Provider implementa SigningKeyProvider sobre Android Keystore.
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
		return nil, errors.New("android-keystore: bridge nativo no configurado")
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
		return nil, errors.New("android-keystore: el bridge nativo no devolvio un identificador de clave")
	}
	return &KeyRef{
		id:    keyID,
		alias: strings.TrimSpace(resolved.Alias),
	}, nil
}

var _ ports.SigningKeyProvider = (*Provider)(nil)
var _ ports.SigningKey = (*KeyRef)(nil)
