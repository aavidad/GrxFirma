// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"io"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/security/signingpolicy"
)

type remoteSigner struct {
	store           *Almacen
	ctx             context.Context
	id, fingerprint string
	der             []byte
}

var _ crypto.Signer = (*remoteSigner)(nil)

func (s *remoteSigner) Public() crypto.PublicKey {
	cert, err := x509.ParseCertificate(append([]byte(nil), s.der...))
	if err != nil {
		return nil
	} // DER is fixed and checked at construction.
	return cert.PublicKey
}

// Sign accepts crypto.Hash options only. PSS, nil (including typed nil), SHA-1
// and custom option types cannot silently select a different mechanism.
// Randomness belongs to the token; the caller's reader is never transmitted.
func (s *remoteSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	hash, ok := opts.(crypto.Hash)
	if !ok {
		return nil, ErrUnsupportedAlgorithm
	}
	name := ""
	switch hash {
	case crypto.SHA256:
		name = "sha256"
	case crypto.SHA384:
		name = "sha384"
	case crypto.SHA512:
		name = "sha512"
	default:
		return nil, ErrUnsupportedAlgorithm
	}
	if len(digest) != hash.Size() {
		return nil, ErrUnsupportedAlgorithm
	}
	if err := signingpolicy.ValidateCertificateDER(s.der, s.store.now()); err != nil {
		return nil, err
	}
	// Preserve the original bytes even if a transport unexpectedly changes
	// its request buffer. Verification never uses helper-controlled input.
	original := append([]byte(nil), digest...)
	defer clear(original)
	requestDigest := append([]byte(nil), original...)
	defer clear(requestDigest)
	response, err := s.store.execute(s.ctx, pkcs11worker.Request{
		Operation: "sign", CertificateID: s.id, Fingerprint: s.fingerprint, Hash: name, Digest: requestDigest,
	})
	if err != nil {
		return nil, err
	}
	if len(response.Certificates) != 0 || len(response.ChainDER) != 0 ||
		len(response.Signature) == 0 || len(response.Signature) > pkcs11worker.MaxSignatureBytes {
		return nil, ErrInvalidResponse
	}
	if err := signingpolicy.ValidateCertificateDER(s.der, s.store.now()); err != nil {
		return nil, err
	}
	signature := append([]byte(nil), response.Signature...)
	valid := false
	switch public := s.Public().(type) {
	case *rsa.PublicKey:
		valid = rsa.VerifyPKCS1v15(public, hash, original, signature) == nil
	case *ecdsa.PublicKey:
		valid = ecdsa.VerifyASN1(public, original, signature)
	}
	if !valid {
		return nil, ErrSignatureInvalid
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	return signature, nil
}
