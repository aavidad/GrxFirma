// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/signingpolicy"
)

type testClock struct{ value time.Time }

func (c *testClock) Now() time.Time { return c.value }

type fakeExecutor struct {
	mu       sync.Mutex
	requests []pkcs11worker.Request
	run      func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error)
}

func (f *fakeExecutor) Execute(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
	f.mu.Lock()
	copy := request
	copy.Digest = append([]byte(nil), request.Digest...)
	f.requests = append(f.requests, copy)
	f.mu.Unlock()
	return f.run(ctx, request)
}
func (f *fakeExecutor) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.requests) }

type fixture struct {
	key       crypto.Signer
	cert      *x509.Certificate
	ref       domain.CertificateRef
	clock     *testClock
	transport *fakeExecutor
	store     *Almacen
}

func newFixture(t *testing.T, algorithm string, mutate func(*x509.Certificate)) *fixture {
	t.Helper()
	var key crypto.Signer
	var err error
	if algorithm == "RSA" {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Identidad sintética QA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	if mutate != nil {
		mutate(template)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	f := &fixture{key: key, cert: cert, clock: &testClock{now}, ref: domain.CertificateRef{
		ID: "pkcs11:v1:synthetic-slot-id", Fingerprint: hex.EncodeToString(sum[:]), Subject: template.Subject.String(), NotAfter: template.NotAfter}}
	f.transport = &fakeExecutor{run: func(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
		if ctx.Err() != nil {
			return pkcs11worker.Response{}, ctx.Err()
		}
		if request.ModulePath != "" || request.ID != "" || request.Version != 0 {
			t.Error("adapter must leave transport-owned fields unset")
		}
		response := pkcs11worker.Response{Code: "ok"}
		switch request.Operation {
		case "list":
			if request.CertificateID != "" || request.Fingerprint != "" || request.Hash != "" || len(request.Digest) != 0 {
				t.Error("list carries signing fields")
			}
			response.Certificates = []pkcs11worker.CatalogEntry{{Certificate: f.ref}}
		case "describe":
			if request.CertificateID != f.ref.ID || request.Fingerprint != f.ref.Fingerprint || request.Hash != "" || len(request.Digest) != 0 {
				t.Error("describe identity changed")
			}
			response.ChainDER = [][]byte{append([]byte(nil), f.cert.Raw...)}
		case "sign":
			if request.CertificateID != f.ref.ID || request.Fingerprint != f.ref.Fingerprint {
				t.Error("sign identity changed")
			}
			hash := map[string]crypto.Hash{"sha256": crypto.SHA256, "sha384": crypto.SHA384, "sha512": crypto.SHA512}[request.Hash]
			if hash == 0 || len(request.Digest) != hash.Size() {
				t.Fatal("invalid sign algorithm")
			}
			var err error
			response.Signature, err = f.key.Sign(rand.Reader, request.Digest, hash)
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unexpected operation %q", request.Operation)
		}
		return response, nil
	}}
	f.store = &Almacen{client: f.transport, clock: f.clock}
	return f
}

func (f *fixture) remote(t *testing.T, ctx context.Context) crypto.Signer {
	t.Helper()
	key, err := f.store.KeyFor(ctx, f.ref)
	if err != nil {
		t.Fatal(err)
	}
	local, ok := key.(*signer.ClaveLocal)
	if !ok {
		t.Fatalf("incompatible key type %T", key)
	}
	return local.ToLocalSigningKey().Signer
}

func TestCatalogPreservesOnlyExplicitKeyEvidence(t *testing.T) {
	f := newFixture(t, "EC", nil)
	for _, evidence := range []bool{false, true} {
		f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) {
			ref := f.ref
			ref.HasSigningKey = !evidence // JSON field is not evidence.
			return pkcs11worker.Response{Code: "ok", Certificates: []pkcs11worker.CatalogEntry{{Certificate: ref, HasSigningKey: evidence}}}, nil
		}
		refs, err := f.store.List(context.Background())
		if err != nil || len(refs) != 1 || refs[0].HasSigningKey != evidence {
			t.Fatalf("catalog=%v err=%v", refs, err)
		}
	}
	if f.transport.count() != 2 {
		t.Fatal("catalog started other operations")
	}
}

func TestRemoteSignerRoundTripAndImmutableIdentity(t *testing.T) {
	for _, algorithm := range []string{"RSA", "EC"} {
		t.Run(algorithm, func(t *testing.T) {
			f := newFixture(t, algorithm, nil)
			remote := f.remote(t, context.Background())
			publicDER, _ := x509.MarshalPKIXPublicKey(remote.Public())
			// Public never returns the retained identity's mutable big integers.
			switch public := remote.Public().(type) {
			case *rsa.PublicKey:
				public.N.SetInt64(1)
			case *ecdsa.PublicKey:
				public.X.SetInt64(1)
			}
			again, _ := x509.MarshalPKIXPublicKey(remote.Public())
			if !bytes.Equal(publicDER, again) {
				t.Fatal("Public mutated identity")
			}
			for _, hash := range []crypto.Hash{crypto.SHA256, crypto.SHA384, crypto.SHA512} {
				h := hash.New()
				h.Write([]byte("payload sintético"))
				digest := h.Sum(nil)
				signature, err := remote.Sign(nil, digest, hash)
				if err != nil {
					t.Fatal(err)
				}
				switch public := f.key.Public().(type) {
				case *rsa.PublicKey:
					if err := rsa.VerifyPKCS1v15(public, hash, digest, signature); err != nil {
						t.Fatal(err)
					}
				case *ecdsa.PublicKey:
					if !ecdsa.VerifyASN1(public, digest, signature) {
						t.Fatal("invalid DER ECDSA")
					}
				}
			}
			if f.transport.count() != 4 {
				t.Fatal("expected describe and three independent signs")
			}
		})
	}
}

func TestKeyForRejectsUnfitDERRegardlessOfCatalogMetadata(t *testing.T) {
	for name, mutate := range map[string]func(*x509.Certificate){
		"expired":   func(c *x509.Certificate) { c.NotAfter = c.NotBefore.Add(time.Minute) },
		"future":    func(c *x509.Certificate) { c.NotBefore = c.NotAfter.Add(-time.Minute) },
		"CA":        func(c *x509.Certificate) { c.IsCA = true },
		"key_usage": func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageKeyEncipherment },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, "EC", mutate)
			f.ref.NotAfter = f.clock.Now().Add(24 * time.Hour)
			_, err := f.store.KeyFor(context.Background(), f.ref)
			if !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) {
				t.Fatalf("error=%v", err)
			}
			if f.transport.count() != 1 {
				t.Fatal("unfit identity reached signing")
			}
		})
	}
}

func TestKeyForRejectsHostileIdentityResponse(t *testing.T) {
	f := newFixture(t, "EC", nil)
	cases := map[string]pkcs11worker.Response{
		"empty":           {Code: "ok"},
		"invalid_der":     {Code: "ok", ChainDER: [][]byte{{1, 2, 3}}},
		"duplicate":       {Code: "ok", ChainDER: [][]byte{f.cert.Raw, f.cert.Raw}},
		"oversize":        {Code: "ok", ChainDER: [][]byte{make([]byte, pkcs11worker.MaxCertificateBytes+1)}},
		"many":            {Code: "ok", ChainDER: make([][]byte, pkcs11worker.MaxChainCertificates+1)},
		"extra_signature": {Code: "ok", ChainDER: [][]byte{f.cert.Raw}, Signature: []byte{1}},
		"bad_code":        {Code: "unrecognized", ChainDER: [][]byte{f.cert.Raw}},
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) { return response, nil }
			if _, err := f.store.KeyFor(context.Background(), f.ref); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) {
		return pkcs11worker.Response{Code: "ok", ChainDER: [][]byte{f.cert.Raw}}, nil
	}
	ref := f.ref
	ref.Fingerprint = strings.Repeat("0", 64)
	if _, err := f.store.KeyFor(context.Background(), ref); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("FP error=%v", err)
	}
	before := f.transport.count()
	for _, ref := range []domain.CertificateRef{{}, {ID: "id", Fingerprint: strings.Repeat("z", 64)}, {ID: strings.Repeat("i", 2049), Fingerprint: f.ref.Fingerprint}} {
		if _, err := f.store.KeyFor(context.Background(), ref); !errors.Is(err, ErrIdentityMismatch) {
			t.Fatalf("ref error=%v", err)
		}
	}
	if f.transport.count() != before {
		t.Fatal("invalid ref crossed process boundary")
	}
}

func TestCatalogRejectsHostileResponse(t *testing.T) {
	f := newFixture(t, "EC", nil)
	badRef := f.ref
	badRef.Fingerprint = "invalid"
	largeRef := f.ref
	largeRef.Subject = strings.Repeat("x", 4097)
	for name, response := range map[string]pkcs11worker.Response{
		"duplicate":    {Code: "ok", Certificates: []pkcs11worker.CatalogEntry{{Certificate: f.ref}, {Certificate: f.ref}}},
		"invalid_ref":  {Code: "ok", Certificates: []pkcs11worker.CatalogEntry{{Certificate: badRef}}},
		"oversize_ref": {Code: "ok", Certificates: []pkcs11worker.CatalogEntry{{Certificate: largeRef}}},
		"many":         {Code: "ok", Certificates: make([]pkcs11worker.CatalogEntry, pkcs11worker.MaxCertificates+1)},
		"chain":        {Code: "ok", ChainDER: [][]byte{f.cert.Raw}},
		"signature":    {Code: "ok", Signature: []byte{1}},
	} {
		t.Run(name, func(t *testing.T) {
			f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) { return response, nil }
			if _, err := f.store.List(context.Background()); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestConstructorsDoNotStartHelperAndNilStoreFailsClosed(t *testing.T) {
	client := pkcs11worker.Client{Executable: "/never-started", ModulePath: "/never-loaded", PINSource: func(context.Context, pkcs11worker.PINMode) ([]byte, error) {
		t.Fatal("unexpected PIN")
		return nil, nil
	}}
	_ = New(client)
	_ = NewWithOptions(client, Options{Clock: &testClock{time.Now()}})
	for _, store := range []*Almacen{nil, {}} {
		if _, err := store.List(context.Background()); !errors.Is(err, pkcs11worker.ErrHelperUnavailable) {
			t.Fatalf("error=%v", err)
		}
	}
}
