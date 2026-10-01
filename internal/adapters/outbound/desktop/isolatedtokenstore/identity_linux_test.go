// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/adapters/outbound/desktop/signer"
)

func TestChainLinkageAndDefensiveCopies(t *testing.T) {
	ca := newFixture(t, "EC", func(c *x509.Certificate) { c.IsCA = true; c.KeyUsage = x509.KeyUsageCertSign })
	f := newFixture(t, "EC", nil)
	der, err := x509.CreateCertificate(rand.Reader, f.cert, ca.cert, f.key.Public(), ca.key)
	if err != nil {
		t.Fatal(err)
	}
	f.cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	f.ref.Fingerprint = hex.EncodeToString(sum[:])
	responseDER := [][]byte{append([]byte(nil), der...), append([]byte(nil), ca.cert.Raw...)}
	validRun := f.transport.run
	f.transport.run = func(ctx context.Context, r pkcs11worker.Request) (pkcs11worker.Response, error) {
		if r.Operation == "describe" {
			return pkcs11worker.Response{Code: "ok", ChainDER: responseDER}, nil
		}
		return validRun(ctx, r)
	}
	key, err := f.store.KeyFor(context.Background(), f.ref)
	if err != nil {
		t.Fatal(err)
	}
	chain := key.CertificateChainDER()
	if len(chain) != 2 || !bytes.Equal(chain[0], der) || !bytes.Equal(chain[1], ca.cert.Raw) {
		t.Fatal("chain not preserved")
	}
	clear(responseDER[0])
	clear(responseDER[1])
	clear(chain[0])
	clear(chain[1])
	chain = key.CertificateChainDER()
	if !bytes.Equal(chain[0], der) || !bytes.Equal(chain[1], ca.cert.Raw) {
		t.Fatal("response/output mutated stored chain")
	}
	local := key.(*signer.ClaveLocal).ToLocalSigningKey()
	// Even a caller mutating the compatibility wrapper's certificate cannot
	// redefine the proxy's fixed identity or public key.
	clear(local.Certificate.Raw)
	if _, err := local.Signer.Sign(nil, make([]byte, 32), crypto.SHA256); err != nil {
		t.Fatal(err)
	}
	unrelated := newFixture(t, "EC", func(c *x509.Certificate) { c.IsCA = true; c.KeyUsage = x509.KeyUsageCertSign })
	if _, err := parseIdentity([][]byte{der, unrelated.cert.Raw}, f.ref.Fingerprint, f.clock.Now()); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("unlinked chain error=%v", err)
	}
}

func TestUnsupportedPublicKeyRejectedBeforeSign(t *testing.T) {
	f := newFixture(t, "EC", nil)
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := *f.cert
	template.SignatureAlgorithm = x509.UnknownSignatureAlgorithm
	template.PublicKey = public
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, public, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	if _, err := parseIdentity([][]byte{der}, hex.EncodeToString(sum[:]), f.clock.Now()); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Fatalf("error=%v", err)
	}
}
