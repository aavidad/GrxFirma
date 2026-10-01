// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package isolatedtokenstore provides a pure-Go certificate catalog and remote
// crypto.Signer for the Linux PKCS#11 helper. No native driver is loaded here.
// Construction does not start a process. Every operation uses a fresh helper;
// no token session, PIN, or process survives a completed operation.
package isolatedtokenstore

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	ErrInvalidResponse      = errors.New("respuesta del auxiliar de certificados no válida")
	ErrIdentityMismatch     = errors.New("la identidad del certificado no coincide")
	ErrUnsupportedAlgorithm = errors.New("algoritmo de firma aislada no soportado")
	ErrSignatureInvalid     = errors.New("firma del auxiliar no válida para el certificado seleccionado")
)

// Options allows a local, trusted clock to be injected; it never changes trust
// anchors or permits bypassing certificate suitability checks.
type Options struct{ Clock ports.Clock }

// executor is deliberately private: production constructors accept only the
// bounded process client, not an arbitrary transport or native signer.
type executor interface {
	Execute(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error)
}

type Almacen struct {
	client executor
	clock  ports.Clock
}

var _ ports.CertificateCatalog = (*Almacen)(nil)
var _ ports.SigningKeyProvider = (*Almacen)(nil)

func New(client pkcs11worker.Client) *Almacen { return NewWithOptions(client, Options{}) }

func NewWithOptions(client pkcs11worker.Client, options Options) *Almacen {
	return &Almacen{client: client, clock: options.Clock}
}

func (a *Almacen) now() time.Time {
	if a.clock != nil {
		return a.clock.Now()
	}
	return time.Now()
}

// List never requests a PIN. Client rejects PIN challenges for list/describe.
// A hidden private object remains HasSigningKey=false; certificate presence
// alone does not prove private-key availability or authorize an unlock prompt.
func (a *Almacen) List(ctx context.Context) ([]domain.CertificateRef, error) {
	response, err := a.execute(ctx, pkcs11worker.Request{Operation: "list"})
	if err != nil {
		return nil, err
	}
	if len(response.Certificates) > pkcs11worker.MaxCertificates || len(response.ChainDER) != 0 || len(response.Signature) != 0 {
		return nil, ErrInvalidResponse
	}
	refs := make([]domain.CertificateRef, 0, len(response.Certificates))
	seen := make(map[string]bool, len(response.Certificates))
	for _, entry := range response.Certificates {
		ref := entry.Certificate
		if entry.HasSigningKey && entry.SigningKeyNeedsUnlock {
			return nil, ErrInvalidResponse
		}
		if !validReference(ref) || seen[ref.ID] || len(ref.Subject) > 4096 || len(ref.Issuer) > 4096 ||
			len(ref.Organizacion) > 4096 || len(ref.NIF) > 512 || len(ref.Tipo) > 128 {
			return nil, ErrInvalidResponse
		}
		seen[ref.ID] = true
		ref.HasSigningKey = entry.HasSigningKey
		ref.SigningKeyNeedsUnlock = entry.SigningKeyNeedsUnlock
		refs = append(refs, ref)
	}
	return refs, nil
}

// KeyFor fixes the exact opaque ID, SHA-256 fingerprint and DER identity for
// subsequent operations. The returned key retains ctx, not a live process.
// ClaveLocal is a compatibility wrapper: its crypto.Signer is remote and never
// contains a private key. The supplied chain is not installed as a trust root.
func (a *Almacen) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validReference(ref) {
		return nil, ErrIdentityMismatch
	}
	response, err := a.execute(ctx, pkcs11worker.Request{
		Operation: "describe", CertificateID: ref.ID, Fingerprint: ref.Fingerprint,
	})
	if err != nil {
		return nil, err
	}
	if len(response.Certificates) != 0 || len(response.Signature) != 0 {
		return nil, ErrInvalidResponse
	}
	chain, err := parseIdentity(response.ChainDER, ref.Fingerprint, a.now())
	if err != nil {
		return nil, err
	}
	remote := &remoteSigner{store: a, ctx: ctx, id: ref.ID, fingerprint: ref.Fingerprint, der: append([]byte(nil), chain[0].Raw...)}
	return signer.NuevaClaveLocalConCadena(remote, chain[0], chain[1:]), nil
}

func validReference(ref domain.CertificateRef) bool {
	if len(ref.ID) == 0 || len(ref.ID) > 2048 || strings.ContainsRune(ref.ID, 0) || len(ref.Fingerprint) != 64 {
		return false
	}
	_, err := hex.DecodeString(ref.Fingerprint)
	return err == nil
}

func (a *Almacen) execute(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
	if err := ctx.Err(); err != nil {
		return pkcs11worker.Response{}, err
	}
	if a == nil || a.client == nil {
		return pkcs11worker.Response{}, pkcs11worker.ErrHelperUnavailable
	}
	response, err := a.client.Execute(ctx, request)
	if ctx.Err() != nil {
		return pkcs11worker.Response{}, ctx.Err()
	}
	if err != nil {
		return pkcs11worker.Response{}, err
	}
	// Client validates version, request-ID correlation and framing before this
	// boundary. Schema/payload checks here are additional defense in depth.
	if response.Code != "ok" {
		return pkcs11worker.Response{}, ErrInvalidResponse
	}
	return response, nil
}
