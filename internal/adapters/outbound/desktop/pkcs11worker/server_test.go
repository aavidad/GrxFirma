// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var fixtureRSA = sync.OnceValues(func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) })

type fakeKey struct {
	crypto.Signer
	chain  [][]byte
	sign   func([]byte, crypto.SignerOpts) ([]byte, error)
	closed bool
}

func (k *fakeKey) KeyID() string                 { return "qa-identity" }
func (k *fakeKey) CertificateChainDER() [][]byte { return k.chain }
func (k *fakeKey) Close()                        { k.closed = true }
func (k *fakeKey) Sign(_ io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	if k.sign != nil {
		return k.sign(d, o)
	}
	return k.Signer.Sign(rand.Reader, d, o)
}

type fakeBackend struct {
	refs       []domain.CertificateRef
	key        ports.SigningKey
	listErr    error
	keyErr     error
	closed     bool
	closePanic bool
	list       func(context.Context) ([]domain.CertificateRef, error)
}

func (b *fakeBackend) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if b.list != nil {
		return b.list(ctx)
	}
	return b.refs, b.listErr
}
func (b *fakeBackend) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return b.key, b.keyErr
}
func (b *fakeBackend) Close() {
	b.closed = true
	if b.closePanic {
		panic("private driver diagnostic")
	}
}

func fixture(t *testing.T) (Request, *fakeBackend, *fakeKey) {
	t.Helper()
	key, err := fixtureRSA()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "PKCS11 synthetic QA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	fp := sha256.Sum256(der)
	r := validRequest("sign")
	r.Fingerprint = hex.EncodeToString(fp[:])
	sum := sha256.Sum256([]byte("synthetic worker document"))
	r.Digest = sum[:]
	k := &fakeKey{Signer: key, chain: [][]byte{der}}
	b := &fakeBackend{key: k, refs: []domain.CertificateRef{{ID: r.CertificateID, Fingerprint: r.Fingerprint, NotAfter: template.NotAfter, HasSigningKey: true}}}
	return r, b, k
}

func execute(t *testing.T, s Server, r Request, pinReplies ...[]byte) (Response, []PINChallenge, []byte) {
	t.Helper()
	var input, output bytes.Buffer
	if err := WriteRequest(&input, r); err != nil {
		t.Fatal(err)
	}
	for _, reply := range pinReplies {
		if err := WriteFrame(&input, FramePINReply, reply, MaxPINBytes+1); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Serve(context.Background(), &input, &output); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), output.Bytes()...)
	var challenges []PINChallenge
	for {
		kind, payload, err := ReadFrame(&output, MaxResponseBytes)
		if err != nil {
			t.Fatal(err)
		}
		if kind == FramePINChallenge {
			var challenge PINChallenge
			if decodeJSON(payload, &challenge) != nil {
				t.Fatal("bad challenge")
			}
			challenges = append(challenges, challenge)
			continue
		}
		if kind != FrameResponse {
			t.Fatal("bad response kind")
		}
		var result Response
		if decodeJSON(payload, &result) != nil {
			t.Fatal("bad response")
		}
		if output.Len() != 0 {
			t.Fatal("trailing data after response")
		}
		if result.ID != r.ID || result.Version != ProtocolVersion {
			t.Fatal("unbound response")
		}
		return result, challenges, raw
	}
}

func TestServerSignsWithBinaryPINAndClosesBeforeResult(t *testing.T) {
	r, b, key := fixture(t)
	var borrowed []byte
	s := Server{Factory: func(_ string, pin PINSource) (Backend, error) {
		key.sign = func(d []byte, o crypto.SignerOpts) ([]byte, error) {
			var err error
			borrowed, err = pin(context.Background(), PINMode{})
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(borrowed, []byte("QA-only-PIN")) {
				return nil, errors.New("bad test PIN")
			}
			return key.Signer.Sign(rand.Reader, d, o)
		}
		return b, nil
	}}
	result, challenges, raw := execute(t, s, r, append([]byte{1}, []byte("QA-only-PIN")...))
	if result.Code != "ok" || len(challenges) != 1 || challenges[0].ID != r.ID {
		t.Fatalf("result=%s challenges=%d", result.Code, len(challenges))
	}
	if err := rsa.VerifyPKCS1v15(key.Public().(*rsa.PublicKey), crypto.SHA256, r.Digest, result.Signature); err != nil {
		t.Fatal(err)
	}
	if !key.closed || !b.closed {
		t.Fatal("native resources not closed")
	}
	if !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
		t.Fatal("PIN buffer retained")
	}
	if bytes.Contains(raw, []byte("QA-only-PIN")) {
		t.Fatal("PIN leaked into output")
	}
}

func TestServerCatalogAndDescriptionNeverPrompt(t *testing.T) {
	for _, operation := range []string{"list", "describe"} {
		t.Run(operation, func(t *testing.T) {
			r, b, _ := fixture(t)
			r.Operation = operation
			r.Digest = nil
			r.Hash = ""
			if operation == "list" {
				r.CertificateID = ""
				r.Fingerprint = ""
				b.refs[0].HasSigningKey = false
				b.refs[0].SigningKeyNeedsUnlock = true
			}
			result, challenges, _ := execute(t, Server{Factory: func(string, PINSource) (Backend, error) { return b, nil }}, r)
			if result.Code != "ok" || len(challenges) != 0 {
				t.Fatal(result.Code)
			}
			if operation == "list" && (len(result.Certificates) != 1 || result.Certificates[0].HasSigningKey || !result.Certificates[0].SigningKeyNeedsUnlock) {
				t.Fatal("unknown key availability turned into true")
			}
			if operation == "describe" && len(result.ChainDER) != 1 {
				t.Fatal("missing certificate DER")
			}
		})
	}
}

func TestServerRejectsIdentityAndCertificateSubstitution(t *testing.T) {
	tests := map[string]func(*Request, *fakeBackend, *fakeKey){
		"missing":           func(r *Request, _ *fakeBackend, _ *fakeKey) { r.CertificateID = "missing" },
		"wrong fingerprint": func(r *Request, _ *fakeBackend, _ *fakeKey) { r.Fingerprint = strings.Repeat("0", 64) },
		"duplicate":         func(_ *Request, b *fakeBackend, _ *fakeKey) { b.refs = append(b.refs, b.refs[0]) },
		"different DER": func(r *Request, b *fakeBackend, _ *fakeKey) {
			r.Fingerprint = strings.Repeat("0", 64)
			b.refs[0].Fingerprint = r.Fingerprint
		},
		"bad DER":     func(_ *Request, _ *fakeBackend, k *fakeKey) { k.chain = [][]byte{{1, 2, 3}} },
		"large chain": func(_ *Request, _ *fakeBackend, k *fakeKey) { k.chain = make([][]byte, MaxChainCertificates+1) },
		"missing key": func(_ *Request, b *fakeBackend, _ *fakeKey) { b.key = nil },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			r, b, k := fixture(t)
			change(&r, b, k)
			called := false
			k.sign = func([]byte, crypto.SignerOpts) ([]byte, error) { called = true; return nil, nil }
			result, challenges, _ := execute(t, Server{Factory: func(string, PINSource) (Backend, error) { return b, nil }}, r)
			if result.Code == "ok" || called || len(challenges) != 0 {
				t.Fatal("invalid identity reached signing")
			}
		})
	}
}

func TestServerPreflightRejectsExpiredWithoutPIN(t *testing.T) {
	r, b, k := fixture(t)
	called := false
	k.sign = func([]byte, crypto.SignerOpts) ([]byte, error) { called = true; return nil, nil }
	result, challenges, _ := execute(t, Server{Factory: func(string, PINSource) (Backend, error) { return b, nil }, Now: func() time.Time { return time.Now().Add(2 * time.Hour) }}, r)
	if result.Code != "unsuitable_certificate" || called || len(challenges) != 0 {
		t.Fatal("expiry guard bypassed")
	}
}

func TestServerPINCancelProtectedPathAndPromptLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    PINMode
		reply   []byte
		prompts int
		want    string
	}{
		{"cancel", PINMode{}, []byte{0}, 1, "pin_cancelled"},
		{"bad marker", PINMode{}, []byte{2}, 1, "invalid_request"},
		{"protected", PINMode{ProtectedAuthenticationPath: true}, []byte{1}, 1, "ok"},
		{"protected secret", PINMode{ProtectedAuthenticationPath: true}, []byte{1, 'x'}, 1, "invalid_request"},
		{"repeated request", PINMode{ContextSpecific: true}, []byte{1, 'x'}, 3, "invalid_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, b, k := fixture(t)
			s := Server{Factory: func(_ string, source PINSource) (Backend, error) {
				k.sign = func(d []byte, o crypto.SignerOpts) ([]byte, error) {
					for i := 0; i < test.prompts; i++ {
						if _, err := source(context.Background(), test.mode); err != nil {
							return nil, err
						}
					}
					return k.Signer.Sign(rand.Reader, d, o)
				}
				return b, nil
			}}
			result, challenges, _ := execute(t, s, r, test.reply, test.reply)
			if result.Code != test.want || len(challenges) > 2 {
				t.Fatalf("code=%s count=%d", result.Code, len(challenges))
			}
			if len(challenges) > 0 && (challenges[0].ProtectedAuthenticationPath != test.mode.ProtectedAuthenticationPath || challenges[0].ContextSpecific != test.mode.ContextSpecific) {
				t.Fatal("PIN mode lost")
			}
		})
	}
}

func TestServerNeverReflectsDriverErrorsOrCleanupPanic(t *testing.T) {
	const secret = "driver secret path /private/example PIN=1234"
	for _, test := range []string{"factory", "list", "sign", "cleanup"} {
		t.Run(test, func(t *testing.T) {
			r, b, k := fixture(t)
			s := Server{ErrorCode: func(error) string { return secret }, Factory: func(string, PINSource) (Backend, error) {
				switch test {
				case "factory":
					return b, errors.New(secret)
				case "list":
					b.listErr = errors.New(secret)
				case "sign":
					k.sign = func([]byte, crypto.SignerOpts) ([]byte, error) { return nil, errors.New(secret) }
				case "cleanup":
					b.closePanic = true
				}
				return b, nil
			}}
			result, _, raw := execute(t, s, r)
			if result.Code != "driver_failed" || len(result.Signature) != 0 || bytes.Contains(raw, []byte(secret)) || !b.closed {
				t.Fatalf("unsafe result code=%s", result.Code)
			}
		})
	}
}

func TestServerInvalidInputDoesNotCreateBackend(t *testing.T) {
	var input, output bytes.Buffer
	_ = WriteFrame(&input, FrameRequest, []byte(`{"version":1,"id":"secret-unbound","pin":"not allowed"}`), MaxRequestBytes)
	called := false
	s := Server{Factory: func(string, PINSource) (Backend, error) { called = true; return nil, nil }}
	if err := s.Serve(context.Background(), &input, &output); err != nil {
		t.Fatal(err)
	}
	if called || bytes.Contains(output.Bytes(), []byte("secret-unbound")) {
		t.Fatal("malformed input reached driver or output")
	}
	_, payload, err := ReadFrame(&output, MaxResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	var result Response
	if decodeJSON(payload, &result) != nil || result.Code != "invalid_request" {
		t.Fatal("wrong error")
	}
}
