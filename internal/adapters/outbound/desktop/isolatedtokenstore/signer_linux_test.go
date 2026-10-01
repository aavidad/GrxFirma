// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/signingpolicy"
)

func TestSignerRejectsOptionsBeforeStartingHelper(t *testing.T) {
	f := newFixture(t, "EC", nil)
	remote := f.remote(t, context.Background())
	var typedNil *rsa.PSSOptions
	for _, opts := range []crypto.SignerOpts{nil, typedNil, &rsa.PSSOptions{Hash: crypto.SHA256}, crypto.SHA1, crypto.Hash(0), crypto.MD5} {
		if _, err := remote.Sign(nil, make([]byte, 32), opts); !errors.Is(err, ErrUnsupportedAlgorithm) {
			t.Fatalf("opts=%T error=%v", opts, err)
		}
	}
	for _, size := range []int{0, 31, 33, 48} {
		if _, err := remote.Sign(nil, make([]byte, size), crypto.SHA256); !errors.Is(err, ErrUnsupportedAlgorithm) {
			t.Fatalf("size=%d error=%v", size, err)
		}
	}
	if f.transport.count() != 1 {
		t.Fatal("invalid options started helper")
	}
}

func TestSignerRejectsMaliciousOutput(t *testing.T) {
	for _, algorithm := range []string{"RSA", "EC"} {
		t.Run(algorithm, func(t *testing.T) {
			f := newFixture(t, algorithm, nil)
			remote := f.remote(t, context.Background())
			validRun := f.transport.run
			for name, mutate := range map[string]func(*pkcs11worker.Request, *pkcs11worker.Response){
				"changed_signature":  func(_ *pkcs11worker.Request, r *pkcs11worker.Response) { r.Signature[0] ^= 1 },
				"trailing_signature": func(_ *pkcs11worker.Request, r *pkcs11worker.Response) { r.Signature = append(r.Signature, 0) },
				"empty":              func(_ *pkcs11worker.Request, r *pkcs11worker.Response) { r.Signature = nil },
				"oversize": func(_ *pkcs11worker.Request, r *pkcs11worker.Response) {
					r.Signature = make([]byte, pkcs11worker.MaxSignatureBytes+1)
				},
				"extra_chain": func(_ *pkcs11worker.Request, r *pkcs11worker.Response) { r.ChainDER = [][]byte{f.cert.Raw} },
			} {
				t.Run(name, func(t *testing.T) {
					f.transport.run = func(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
						response, err := validRun(ctx, request)
						mutate(&request, &response)
						return response, err
					}
					if signature, err := remote.Sign(nil, make([]byte, 32), crypto.SHA256); err == nil || signature != nil {
						t.Fatal("hostile response accepted")
					}
				})
			}
			f.transport.run = func(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
				request.Digest[0] ^= 1 // valid signature over the wrong bytes must fail.
				return validRun(ctx, request)
			}
			digest := make([]byte, 32)
			if _, err := remote.Sign(nil, digest, crypto.SHA256); !errors.Is(err, ErrSignatureInvalid) {
				t.Fatalf("digest substitution error=%v", err)
			}
			if !bytes.Equal(digest, make([]byte, 32)) {
				t.Fatal("transport mutated caller digest")
			}
			other := newFixture(t, algorithm, nil)
			f.transport.run = func(_ context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
				sig, err := other.key.Sign(rand.Reader, request.Digest, crypto.SHA256)
				return pkcs11worker.Response{Code: "ok", Signature: sig}, err
			}
			if _, err := remote.Sign(nil, digest, crypto.SHA256); !errors.Is(err, ErrSignatureInvalid) {
				t.Fatalf("wrong key error=%v", err)
			}
		})
	}
}

func TestRetainedContextCancelsAllSubsequentSignatures(t *testing.T) {
	f := newFixture(t, "EC", nil)
	ctx, cancel := context.WithCancel(context.Background())
	remote := f.remote(t, ctx)
	cancel()
	if _, err := remote.Sign(nil, make([]byte, 32), crypto.SHA256); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if _, err := f.store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("list error=%v", err)
	}
	if _, err := f.store.KeyFor(ctx, f.ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("key error=%v", err)
	}
	if f.transport.count() != 1 {
		t.Fatal("cancelled request started helper")
	}
}

func TestCancellationDuringHelperDiscardsResult(t *testing.T) {
	for _, operation := range []string{"list", "describe", "sign"} {
		t.Run(operation, func(t *testing.T) {
			f := newFixture(t, "EC", nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote := f.remote(t, ctx)
			validRun := f.transport.run
			f.transport.run = func(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
				response, err := validRun(ctx, request)
				cancel()
				return response, err
			}
			var err error
			switch operation {
			case "list":
				_, err = f.store.List(ctx)
			case "describe":
				_, err = f.store.KeyFor(ctx, f.ref)
			case "sign":
				_, err = remote.Sign(nil, make([]byte, 32), crypto.SHA256)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestExpiryBeforeAndDuringHelper(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "during"}[during], func(t *testing.T) {
			f := newFixture(t, "EC", nil)
			remote := f.remote(t, context.Background())
			if during {
				validRun := f.transport.run
				f.transport.run = func(ctx context.Context, request pkcs11worker.Request) (pkcs11worker.Response, error) {
					response, err := validRun(ctx, request)
					f.clock.value = f.clock.value.Add(2 * time.Hour)
					return response, err
				}
			} else {
				f.clock.value = f.clock.value.Add(2 * time.Hour)
			}
			if _, err := remote.Sign(nil, make([]byte, 32), crypto.SHA256); !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) {
				t.Fatalf("error=%v", err)
			}
			want := 1
			if during {
				want = 2
			}
			if f.transport.count() != want {
				t.Fatal("unexpected helper count")
			}
		})
	}
}

func TestDriverErrorRetainsTypedCode(t *testing.T) {
	f := newFixture(t, "EC", nil)
	remote := f.remote(t, context.Background())
	expected := &pkcs11worker.OperationError{Code: "pin_locked"}
	f.transport.run = func(context.Context, pkcs11worker.Request) (pkcs11worker.Response, error) {
		return pkcs11worker.Response{}, expected
	}
	if _, err := remote.Sign(nil, make([]byte, 32), crypto.SHA256); err != expected {
		t.Fatalf("error=%v", err)
	}
}

func TestConcurrentSignaturesHaveNoSharedMutableState(t *testing.T) {
	f := newFixture(t, "EC", nil)
	remote := f.remote(t, context.Background())
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			digest := sha256.Sum256([]byte("QA paralelo"))
			if _, err := remote.Sign(nil, digest[:], crypto.SHA256); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.transport.count() != 13 {
		t.Fatal("expected one describe and twelve independent requests")
	}
}

func TestCurrentMotorUsesRemoteSignerWithoutTrustingSelfSignedQA(t *testing.T) {
	for _, algorithm := range []string{"RSA", "EC"} {
		t.Run(algorithm, func(t *testing.T) {
			f := newFixture(t, algorithm, nil)
			key, err := f.store.KeyFor(context.Background(), f.ref)
			if err != nil {
				t.Fatal(err)
			}
			payload := []byte("documento sintético del motor con firma aislada")
			doc, err := domain.NewDocument("qa.txt", payload, "text/plain")
			if err != nil {
				t.Fatal(err)
			}
			result, err := signer.NuevoMotorFirmaGo(f.clock).Sign(context.Background(), domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}, key)
			if err != nil {
				t.Fatal(err)
			}
			verified, _, err := commonsigner.NewCAdESVerifier().VerifyDetachedCMS(context.Background(), result.Data, payload)
			if err != nil || verified.Integrity.Status != domain.VerificationStatusValid {
				t.Fatalf("verification=%+v err=%v", verified, err)
			}
			if verified.Trust.Status == domain.VerificationStatusValid {
				t.Fatal("self-signed helper certificate became a trust anchor")
			}
			if f.transport.count() != 2 {
				t.Fatal("motor did not issue exactly one remote sign")
			}
		})
	}
}
