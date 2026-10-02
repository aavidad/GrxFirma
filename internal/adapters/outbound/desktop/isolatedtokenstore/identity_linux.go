// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/security/signingpolicy"
)

func parseIdentity(raw [][]byte, fingerprint string, now time.Time) ([]*x509.Certificate, error) {
	if len(raw) == 0 || len(raw) > pkcs11worker.MaxChainCertificates {
		return nil, ErrInvalidResponse
	}
	chain := make([]*x509.Certificate, 0, len(raw))
	seen := make(map[[32]byte]bool, len(raw))
	for _, der := range raw {
		if len(der) == 0 || len(der) > pkcs11worker.MaxCertificateBytes {
			return nil, ErrInvalidResponse
		}
		sum := sha256.Sum256(der)
		if seen[sum] {
			return nil, ErrInvalidResponse
		}
		seen[sum] = true
		// Parse a copy: helper response buffers and the public compatibility
		// wrapper must not mutate the identity retained by remoteSigner.
		cert, err := x509.ParseCertificate(append([]byte(nil), der...))
		if err != nil {
			return nil, ErrInvalidResponse
		}
		if len(chain) == 0 {
			if !strings.EqualFold(hex.EncodeToString(sum[:]), fingerprint) {
				return nil, ErrIdentityMismatch
			}
			if err := signingpolicy.ValidateCertificateDER(cert.Raw, now); err != nil {
				return nil, err
			}
			if !supportedPublicKey(cert.PublicKey) {
				return nil, ErrUnsupportedAlgorithm
			}
		} else if chain[len(chain)-1].CheckSignatureFrom(cert) != nil {
			// Check only internal chain linkage, not trust. No system or
			// helper-supplied root is promoted to a trust anchor.
			return nil, ErrInvalidResponse
		}
		chain = append(chain, cert)
	}
	return chain, nil
}

func supportedPublicKey(public crypto.PublicKey) bool {
	switch key := public.(type) {
	case *rsa.PublicKey:
		return key != nil && key.N != nil && key.N.BitLen() >= 2048 && key.N.BitLen() <= 8192 && key.E >= 3 && key.E%2 == 1
	case *ecdsa.PublicKey:
		if key == nil || key.Curve == nil || key.Curve.Params().BitSize < 256 || key.Curve.Params().BitSize > 521 {
			return false
		}
		encoded, err := key.Bytes()
		if err != nil {
			return false
		}
		_, err = ecdsa.ParseUncompressedPublicKey(key.Curve, encoded)
		return err == nil
	default:
		return false
	}
}
