// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11store

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"io"
	"math/big"
	"sync/atomic"

	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

// SigningIdentity es una referencia opaca. No retiene PIN ni sesiones abiertas
// entre firmas. El auxiliar debe mantener vivo el contexto recibido en KeyFor.
type SigningIdentity struct {
	store    *Almacen
	identity identity
	ctx      context.Context
	closed   atomic.Bool
}

func (k *SigningIdentity) KeyID() string { return k.identity.ref.ID }
func (k *SigningIdentity) CertificateChainDER() [][]byte {
	return [][]byte{append([]byte(nil), k.identity.der...)}
}
func (k *SigningIdentity) Public() crypto.PublicKey {
	cert, err := x509.ParseCertificate(k.identity.der)
	if err != nil {
		return nil
	}
	return cert.PublicKey
}
func (k *SigningIdentity) Close() {
	if k != nil {
		k.closed.Store(true)
	}
}

func (k *SigningIdentity) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if k == nil || k.closed.Load() {
		return nil, ErrClosed
	}
	if err := k.ctx.Err(); err != nil {
		return nil, err
	}
	algorithm, input, err := prepareSignature(k.Public(), digest, opts)
	if err != nil {
		return nil, err
	}
	a := k.store
	if err := a.acquire(k.ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if k.closed.Load() {
		return nil, ErrClosed
	}
	if err := a.initialize(); err != nil {
		return nil, err
	}
	if err := signingpolicy.ValidateCertificateDER(k.identity.der, a.now()); err != nil {
		return nil, err
	}
	return k.sign(algorithm, input, digest, opts.HashFunc())
}

func (k *SigningIdentity) sign(algorithm mechanism, input, digest []byte, hash crypto.Hash) (result []byte, err error) {
	a, ctx, expected := k.store, k.ctx, k.identity
	token, err := a.driver.TokenInfo(expected.slot)
	if err != nil {
		return nil, err
	}
	if a.identityID(expected.slot, token, expected.objectID, expected.ref.Fingerprint) != expected.ref.ID {
		return nil, ErrIdentityNotFound
	}
	available, err := a.driver.Mechanisms(expected.slot)
	if err != nil {
		return nil, err
	}
	supported := false
	for _, candidate := range available {
		if candidate == algorithm {
			supported = true
		}
	}
	if !supported {
		return nil, ErrMechanismUnsupported
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := a.driver.OpenSession(expected.slot)
	if err != nil {
		return nil, err
	}
	loggedIn := false
	defer func() {
		if loggedIn {
			err = errors.Join(err, a.driver.Logout(session))
		}
		err = errors.Join(err, a.driver.CloseSession(session))
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		if k.closed.Load() {
			err = errors.Join(err, ErrClosed)
		}
		if err != nil {
			clear(result)
			result = nil
		}
	}()
	// Releer el certificado antes de cualquier PIN o firma impide reutilizar una
	// referencia tras sustituir token/certificado o reasignar CKA_ID.
	certificates, err := a.find(ctx, session, objectQuery{id: expected.objectID})
	if err != nil {
		return nil, err
	}
	if len(certificates) != 1 {
		return nil, ErrAmbiguousIdentity
	}
	der, objectID, err := a.driver.Certificate(session, certificates[0])
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(der, expected.der) || !bytes.Equal(objectID, expected.objectID) {
		return nil, ErrIdentityNotFound
	}
	if token.loginRequired {
		loggedIn, err = a.login(ctx, session, expected.slot, token, false)
		if err != nil {
			return nil, err
		}
	}
	keys, err := a.find(ctx, session, objectQuery{private: true, id: expected.objectID, signing: true})
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrIdentityNotFound
	}
	if len(keys) != 1 {
		return nil, ErrAmbiguousIdentity
	}
	always, err := a.driver.AlwaysAuthenticate(session, keys[0])
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := a.driver.SignInit(session, algorithm, keys[0]); err != nil {
		return nil, err
	}
	if always {
		if _, err := a.login(ctx, session, expected.slot, token, true); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k.closed.Load() {
		return nil, ErrClosed
	}
	// El tiempo de interacción con el PIN no prolonga la vigencia del certificado.
	if err := signingpolicy.ValidateCertificateDER(expected.der, a.now()); err != nil {
		return nil, err
	}
	raw, err := a.driver.Sign(session, input)
	if err != nil {
		clear(raw)
		return nil, err
	}
	defer clear(raw)
	return validateSignature(k.Public(), hash, digest, raw)
}

func (a *Almacen) login(ctx context.Context, session sessionHandle, slot uint, token tokenInfo, specific bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.options.PINProvider == nil {
		return false, ErrPINUnavailable
	}
	pin, err := a.options.PINProvider.RequestPIN(ctx, PINRequest{Token: a.tokenRef(slot, token), ProtectedAuthenticationPath: token.protectedAuthentication, ContextSpecific: specific})
	defer clear(pin)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, ErrPINUnavailable // el mensaje libre del proveedor podría contener un secreto
	}
	if token.protectedAuthentication {
		if len(pin) != 0 {
			return false, ErrPINUnavailable
		}
	} else if len(pin) == 0 || len(pin) > maxPINBytes || uint(len(pin)) < token.minPIN || (token.maxPIN != 0 && uint(len(pin)) > token.maxPIN) {
		return false, ErrPINUnavailable
	}
	err = a.driver.Login(session, pin, specific)
	if errors.Is(err, errAlreadyLoggedIn) && !specific {
		return false, nil
	}
	return err == nil && !specific, err
}

func prepareSignature(public crypto.PublicKey, digest []byte, opts crypto.SignerOpts) (mechanism, []byte, error) {
	if opts == nil {
		return 0, nil, ErrMechanismUnsupported
	}
	if _, pss := opts.(*rsa.PSSOptions); pss {
		return 0, nil, ErrMechanismUnsupported
	}
	hash := opts.HashFunc()
	var oid asn1.ObjectIdentifier
	switch hash {
	case crypto.SHA256:
		oid = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	case crypto.SHA384:
		oid = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	case crypto.SHA512:
		oid = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	default:
		return 0, nil, ErrMechanismUnsupported
	}
	if len(digest) != hash.Size() {
		return 0, nil, ErrMechanismUnsupported
	}
	switch key := public.(type) {
	case *rsa.PublicKey:
		if key == nil || key.N == nil || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 {
			return 0, nil, ErrMechanismUnsupported
		}
		// CKM_RSA_PKCS aplica el padding, pero necesita DigestInfo completo.
		input, err := asn1.Marshal(struct {
			Algorithm struct {
				OID        asn1.ObjectIdentifier
				Parameters asn1.RawValue
			}
			Digest []byte
		}{Algorithm: struct {
			OID        asn1.ObjectIdentifier
			Parameters asn1.RawValue
		}{oid, asn1.RawValue{FullBytes: []byte{5, 0}}}, Digest: digest})
		return rsaPKCS, input, err
	case *ecdsa.PublicKey:
		if key == nil || key.Curve == nil || key.Curve.Params().BitSize < 256 || key.Curve.Params().BitSize > 521 {
			return 0, nil, ErrMechanismUnsupported
		}
		return ecdsaRaw, append([]byte(nil), digest...), nil
	default:
		return 0, nil, ErrMechanismUnsupported
	}
}

func validateSignature(public crypto.PublicKey, hash crypto.Hash, digest, raw []byte) ([]byte, error) {
	switch key := public.(type) {
	case *rsa.PublicKey:
		if len(raw) != key.Size() || rsa.VerifyPKCS1v15(key, hash, digest, raw) != nil {
			return nil, ErrSignatureInvalid
		}
		return append([]byte(nil), raw...), nil
	case *ecdsa.PublicKey:
		width := (key.Curve.Params().BitSize + 7) / 8
		if len(raw) != 2*width {
			return nil, ErrSignatureInvalid
		}
		r, s := new(big.Int).SetBytes(raw[:width]), new(big.Int).SetBytes(raw[width:])
		if !ecdsa.Verify(key, digest, r, s) {
			return nil, ErrSignatureInvalid
		}
		return asn1.Marshal(struct{ R, S *big.Int }{r, s})
	default:
		return nil, ErrMechanismUnsupported
	}
}

var _ crypto.Signer = (*SigningIdentity)(nil)
var _ ports.SigningKey = (*SigningIdentity)(nil)
