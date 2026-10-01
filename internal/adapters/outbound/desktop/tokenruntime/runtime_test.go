// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type fakeKey struct{}

func (fakeKey) KeyID() string                 { return "synthetic-key" }
func (fakeKey) CertificateChainDER() [][]byte { return nil }

type fakeSource struct {
	refs   []domain.CertificateRef
	err    error
	pin    pkcs11worker.PINSource
	keyFor func(context.Context, domain.CertificateRef, pkcs11worker.PINSource) (ports.SigningKey, error)
}

func (s *fakeSource) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if s.pin != nil {
		return nil, errors.New("list was supplied a PIN callback")
	}
	return append([]domain.CertificateRef(nil), s.refs...), s.err
}
func (s *fakeSource) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	return s.keyFor(ctx, ref, s.pin)
}

func testRef(id string) domain.CertificateRef {
	return domain.CertificateRef{ID: id, Fingerprint: strings.Repeat("a", 64), Subject: "Identidad local sintética", SigningKeyNeedsUnlock: true}
}

func TestRoutingRequiresObservedExactIdentityAndOriginalPINReference(t *testing.T) {
	var calls []string
	var pinCalls int
	refs := map[string]domain.CertificateRef{"one": testRef("slot-one"), "two": testRef("slot-two")}
	r := &Runtime{modules: []string{"one", "two"}, routes: make(map[string]route)}
	r.prompt = func(ctx context.Context, ref domain.CertificateRef, mode pkcs11worker.PINMode) ([]byte, error) {
		pinCalls++
		if ref.ID != refs["two"].ID || !mode.ContextSpecific {
			t.Fatal("PIN bound to a caller-controlled or incorrect identity")
		}
		return []byte("QA-only"), nil
	}
	r.factory = func(module string, pin pkcs11worker.PINSource) source {
		calls = append(calls, module)
		return &fakeSource{refs: []domain.CertificateRef{refs[module]}, pin: pin, keyFor: func(ctx context.Context, ref domain.CertificateRef, pin pkcs11worker.PINSource) (ports.SigningKey, error) {
			if module != "two" || ref.ID != refs["two"].ID {
				t.Fatal("wrong module probed")
			}
			buffer, err := pin(ctx, pkcs11worker.PINMode{ContextSpecific: true})
			defer clear(buffer)
			if err != nil {
				t.Fatal(err)
			}
			return fakeKey{}, nil
		}}
	}
	if _, err := r.KeyFor(context.Background(), refs["two"]); !errors.Is(err, ErrIdentityUnknown) {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatal("KeyFor probed unseen identity")
	}
	listed, err := r.List(context.Background())
	if err != nil || len(listed) != 2 || pinCalls != 0 {
		t.Fatalf("catalog=%v err=%v", listed, err)
	}
	if !listed[0].SigningKeyNeedsUnlock || listed[0].HasSigningKey {
		t.Fatal("unlock became key evidence")
	}
	forged := refs["two"]
	forged.Subject = "texto desde web"
	forged.HasSigningKey = true
	forged.SigningKeyNeedsUnlock = false
	if _, err := r.KeyFor(context.Background(), forged); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "one,two,two" || pinCalls != 1 {
		t.Fatalf("calls=%v PIN=%d", calls, pinCalls)
	}
	before := len(calls)
	for _, ref := range []domain.CertificateRef{{}, testRef("not-catalogued"), {ID: refs["two"].ID, Fingerprint: strings.Repeat("b", 64)}} {
		if _, err := r.KeyFor(context.Background(), ref); !errors.Is(err, ErrIdentityUnknown) {
			t.Fatal(err)
		}
	}
	if len(calls) != before {
		t.Fatal("unknown identity probed other modules")
	}
}

func TestPartialCatalogKeepsGoodModulesWithSanitizedDiagnostics(t *testing.T) {
	fail := map[string]bool{"one": true}
	r := &Runtime{modules: []string{"one", "two"}}
	r.factory = func(module string, pin pkcs11worker.PINSource) source {
		s := &fakeSource{refs: []domain.CertificateRef{testRef(module)}, pin: pin}
		if fail[module] {
			s.err = errors.New("/secret/module-path driver-secret PIN-value")
		}
		return s
	}
	refs, err := r.List(context.Background())
	if err != nil || len(refs) != 1 || refs[0].ID != "two" {
		t.Fatalf("catalog=%v err=%v", refs, err)
	}
	diag := r.Diagnostico()
	if !errors.Is(diag, ErrCatalogUnavailable) || !strings.Contains(diag.Error(), "módulo local 1") {
		t.Fatal(diag)
	}
	if strings.Contains(diag.Error(), "secret") || strings.Contains(diag.Error(), "PIN-value") {
		t.Fatal("driver error leaked")
	}
	fail["two"] = true
	if _, err := r.List(context.Background()); !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatal(err)
	}
	if _, err := r.KeyFor(context.Background(), testRef("two")); !errors.Is(err, ErrIdentityUnknown) {
		t.Fatal("old route retained after total failure")
	}
	delete(fail, "one")
	delete(fail, "two")
	if _, err := r.List(context.Background()); err != nil || r.Diagnostico() != nil {
		t.Fatal("diagnostic did not recover")
	}
}

func TestIdentityCollisionDoesNotSelectAnyModule(t *testing.T) {
	r := &Runtime{modules: []string{"one", "two"}, factory: func(string, pkcs11worker.PINSource) source {
		return &fakeSource{refs: []domain.CertificateRef{testRef("duplicate")}}
	}}
	refs, err := r.List(context.Background())
	if err != nil || len(refs) != 0 || r.Diagnostico() == nil {
		t.Fatalf("catalog=%v err=%v", refs, err)
	}
	if _, err := r.KeyFor(context.Background(), testRef("duplicate")); !errors.Is(err, ErrIdentityUnknown) {
		t.Fatal("ambiguous module selected")
	}
}

func TestMissingPromptAndCancelledContextNeverInventPIN(t *testing.T) {
	ref := testRef("one")
	r := &Runtime{routes: map[string]route{ref.ID: {module: "one", reference: ref}}}
	r.factory = func(_ string, pin pkcs11worker.PINSource) source {
		return &fakeSource{pin: pin, keyFor: func(ctx context.Context, _ domain.CertificateRef, pin pkcs11worker.PINSource) (ports.SigningKey, error) {
			_, err := pin(ctx, pkcs11worker.PINMode{})
			return nil, err
		}}
	}
	if _, err := r.KeyFor(context.Background(), ref); !errors.Is(err, pkcs11worker.ErrPINCancelled) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.KeyFor(ctx, ref); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := r.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestConcurrentCatalogAndKeyLookup(t *testing.T) {
	ref := testRef("one")
	r := &Runtime{modules: []string{"one"}, factory: func(_ string, pin pkcs11worker.PINSource) source {
		return &fakeSource{refs: []domain.CertificateRef{ref}, pin: pin, keyFor: func(context.Context, domain.CertificateRef, pkcs11worker.PINSource) (ports.SigningKey, error) {
			return fakeKey{}, nil
		}}
	}}
	if _, err := r.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.List(context.Background()); err != nil {
				t.Error(err)
			}
			if _, err := r.KeyFor(context.Background(), ref); err != nil {
				t.Error(err)
			}
			_ = r.Diagnostico()
		}()
	}
	wg.Wait()
}
