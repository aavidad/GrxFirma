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
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/signingpolicy"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeObject struct {
	handle          objectHandle
	slot            uint
	id, der         []byte
	private         crypto.Signer
	signing, always bool
}
type fakeSession struct {
	slot                uint
	search              bool
	results             []objectHandle
	logged, initialized bool
	key                 objectHandle
	algorithm           mechanism
}

// Este doble falla ante sesiones inexistentes, búsquedas solapadas, firmas sin
// SignInit o cierre con búsqueda activa. No accede a tokens del equipo.
type fakeModule struct {
	t                                                                                     *testing.T
	tokens                                                                                map[uint]tokenInfo
	objects                                                                               []fakeObject
	sessions                                                                              map[sessionHandle]*fakeSession
	next                                                                                  sessionHandle
	loads, initializes, finalizes, destroys, opens, closes, finds, logins, logouts, signs int
	mechanisms                                                                            []mechanism
	loginError, signError, closeError, findError, finalError                              error
	hidePrivate                                                                           bool
	corruptSignature                                                                      bool
	loginHook                                                                             func([]byte, bool)
	signHook                                                                              func()
}

func newFakeModule(t *testing.T) *fakeModule {
	return &fakeModule{t: t, tokens: map[uint]tokenInfo{1: {label: "Token QA", serial: "sintetico-1", initialized: true, loginRequired: true, minPIN: 4, maxPIN: 8}}, sessions: make(map[sessionHandle]*fakeSession), mechanisms: []mechanism{rsaPKCS, ecdsaRaw}}
}
func (m *fakeModule) session(id sessionHandle) *fakeSession {
	s, ok := m.sessions[id]
	if !ok {
		m.t.Fatalf("sesión inexistente %d", id)
	}
	return s
}
func (m *fakeModule) Initialize() error { m.initializes++; return nil }
func (m *fakeModule) Finalize() error {
	if len(m.sessions) != 0 {
		m.t.Fatal("finalización con sesiones abiertas")
	}
	m.finalizes++
	return nil
}
func (m *fakeModule) Destroy() { m.destroys++ }
func (m *fakeModule) Slots() ([]uint, error) {
	slots := make([]uint, 0, len(m.tokens))
	for slot := range m.tokens {
		slots = append(slots, slot)
	}
	return slots, nil
}
func (m *fakeModule) TokenInfo(slot uint) (tokenInfo, error) {
	info, ok := m.tokens[slot]
	if !ok {
		return tokenInfo{}, ErrTokenUnavailable
	}
	return info, nil
}
func (m *fakeModule) OpenSession(slot uint) (sessionHandle, error) {
	m.next++
	m.opens++
	m.sessions[m.next] = &fakeSession{slot: slot}
	return m.next, nil
}
func (m *fakeModule) CloseSession(id sessionHandle) error {
	s := m.session(id)
	if s.search {
		m.t.Fatal("cierre durante búsqueda")
	}
	delete(m.sessions, id)
	m.closes++
	return m.closeError
}
func (m *fakeModule) FindInit(id sessionHandle, q objectQuery) error {
	s := m.session(id)
	if s.search {
		m.t.Fatal("búsquedas solapadas")
	}
	s.search = true
	s.results = nil
	for _, o := range m.objects {
		if o.slot != s.slot || (o.private != nil) != q.private || (q.id != nil && !bytes.Equal(q.id, o.id)) || (q.signing && !o.signing) {
			continue
		}
		if q.private && m.hidePrivate && !s.logged {
			continue
		}
		s.results = append(s.results, o.handle)
	}
	return nil
}
func (m *fakeModule) Find(id sessionHandle, count int) ([]objectHandle, error) {
	s := m.session(id)
	if !s.search {
		m.t.Fatal("Find sin FindInit")
	}
	m.finds++
	if m.findError != nil {
		return nil, m.findError
	}
	if count > len(s.results) {
		count = len(s.results)
	}
	result := append([]objectHandle(nil), s.results[:count]...)
	s.results = s.results[count:]
	return result, nil
}
func (m *fakeModule) FindFinal(id sessionHandle) error {
	s := m.session(id)
	if !s.search {
		m.t.Fatal("FindFinal sin búsqueda")
	}
	s.search = false
	return m.finalError
}
func (m *fakeModule) object(id sessionHandle, handle objectHandle) fakeObject {
	s := m.session(id)
	for _, o := range m.objects {
		if o.handle == handle && o.slot == s.slot {
			return o
		}
	}
	m.t.Fatalf("objeto %d no corresponde a sesión", handle)
	return fakeObject{}
}
func (m *fakeModule) Certificate(id sessionHandle, h objectHandle) ([]byte, []byte, error) {
	o := m.object(id, h)
	if o.private != nil {
		m.t.Fatal("certificado pedido a clave privada")
	}
	return o.der, o.id, nil
}
func (m *fakeModule) AlwaysAuthenticate(id sessionHandle, h objectHandle) (bool, error) {
	return m.object(id, h).always, nil
}
func (m *fakeModule) Mechanisms(uint) ([]mechanism, error) { return m.mechanisms, nil }
func (m *fakeModule) Login(id sessionHandle, pin []byte, specific bool) error {
	s := m.session(id)
	m.logins++
	if specific && !s.initialized {
		m.t.Fatal("context login antes de SignInit")
	}
	if m.loginHook != nil {
		m.loginHook(pin, specific)
	}
	if m.loginError != nil {
		if errors.Is(m.loginError, errAlreadyLoggedIn) {
			s.logged = true
		}
		return m.loginError
	}
	if !specific {
		s.logged = true
	}
	return nil
}
func (m *fakeModule) Logout(id sessionHandle) error {
	s := m.session(id)
	if !s.logged {
		m.t.Fatal("logout no poseído")
	}
	s.logged = false
	m.logouts++
	return nil
}
func (m *fakeModule) SignInit(id sessionHandle, algorithm mechanism, h objectHandle) error {
	s := m.session(id)
	o := m.object(id, h)
	if o.private == nil || !o.signing {
		m.t.Fatal("clave no firmable")
	}
	if m.tokens[s.slot].loginRequired && !s.logged {
		m.t.Fatal("SignInit sin login")
	}
	s.initialized = true
	s.algorithm = algorithm
	s.key = h
	return nil
}
func (m *fakeModule) Sign(id sessionHandle, input []byte) ([]byte, error) {
	s := m.session(id)
	if !s.initialized {
		m.t.Fatal("Sign sin SignInit")
	}
	m.signs++
	if m.signHook != nil {
		m.signHook()
	}
	if m.signError != nil {
		return nil, m.signError
	}
	o := m.object(id, s.key)
	if m.corruptSignature {
		return []byte("firma no válida"), nil
	}
	switch key := o.private.(type) {
	case *rsa.PrivateKey:
		if s.algorithm != rsaPKCS {
			m.t.Fatal("mecanismo RSA incorrecto")
		}
		return rsa.SignPKCS1v15(rand.Reader, key, 0, input)
	case *ecdsa.PrivateKey:
		if s.algorithm != ecdsaRaw {
			m.t.Fatal("mecanismo EC incorrecto")
		}
		r, v, err := ecdsa.Sign(rand.Reader, key, input)
		if err != nil {
			return nil, err
		}
		width := (key.Curve.Params().BitSize + 7) / 8
		raw := make([]byte, 2*width)
		r.FillBytes(raw[:width])
		v.FillBytes(raw[width:])
		return raw, nil
	default:
		m.t.Fatal("tipo de clave sintética desconocido")
		return nil, nil
	}
}

func addIdentity(t *testing.T, m *fakeModule, slot uint, id byte, private crypto.Signer, now time.Time) {
	t.Helper()
	template := &x509.Certificate{SerialNumber: big.NewInt(int64(id) + 1), Subject: pkix.Name{CommonName: "Identidad sintética QA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, private.Public(), private)
	if err != nil {
		t.Fatal(err)
	}
	base := objectHandle(len(m.objects) + 1)
	m.objects = append(m.objects, fakeObject{handle: base, slot: slot, id: []byte{id}, der: der}, fakeObject{handle: base + 1, slot: slot, id: []byte{id}, private: private, signing: true})
}
func ecdsaKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
func testStore(t *testing.T, m *fakeModule, options Options) *Almacen {
	t.Helper()
	store := NewWithOptions("/modulo/sintetico.so", options)
	store.load = func(string) (module, error) { m.loads++; return m, nil }
	t.Cleanup(func() {
		store.Cerrar()
		if m.opens != m.closes || len(m.sessions) != 0 {
			t.Errorf("sesiones no cerradas: %d/%d", m.opens, m.closes)
		}
	})
	return store
}
func selectedKey(t *testing.T, store *Almacen, ctx context.Context) *SigningIdentity {
	t.Helper()
	refs, err := store.List(ctx)
	if err != nil || len(refs) == 0 {
		t.Fatalf("List: %v, %v", refs, err)
	}
	key, err := store.KeyFor(ctx, refs[len(refs)-1])
	if err != nil {
		t.Fatal(err)
	}
	return key.(*SigningIdentity)
}
func testPIN(context.Context, PINRequest) ([]byte, error) { return []byte("1234"), nil }

func TestCatalogueLazyPaginationAndPreciseSelection(t *testing.T) {
	now := time.Now()
	m := newFakeModule(t)
	private := ecdsaKey(t)
	other := ecdsaKey(t)
	for i := 0; i < 35; i++ {
		key := other
		if i == 34 {
			key = private
		}
		addIdentity(t, m, 1, byte(i), key, now)
	}
	store := testStore(t, m, Options{PINProvider: PINProviderFunc(testPIN)})
	if m.loads != 0 {
		t.Fatal("constructor cargó módulo")
	}
	refs, err := store.List(context.Background())
	if err != nil || len(refs) != 35 {
		t.Fatalf("paginación: %d, %v", len(refs), err)
	}
	if m.logins != 0 {
		t.Fatal("List solicitó login")
	}
	if !refs[34].HasSigningKey {
		t.Fatal("asociación visible no declarada")
	}
	key, err := store.KeyFor(context.Background(), refs[34])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("selección concreta"))
	sig, err := key.(*SigningIdentity).Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil || !ecdsa.VerifyASN1(&private.PublicKey, digest[:], sig) {
		t.Fatalf("firma: %v", err)
	}
	if m.signs != 1 || m.logins != 1 || m.logouts != 1 {
		t.Fatalf("ciclo: sign=%d login=%d logout=%d", m.signs, m.logins, m.logouts)
	}
	if m.loads != 1 || m.initializes != 1 {
		t.Fatal("módulo inicializado más de una vez")
	}
}

func TestRSADigestInfoAndECDSADER(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []crypto.Signer{rsaKey, ecdsaKey(t)} {
		t.Run(map[bool]string{true: "RSA", false: "EC"}[private == rsaKey], func(t *testing.T) {
			m := newFakeModule(t)
			addIdentity(t, m, 1, 1, private, time.Now())
			store := testStore(t, m, Options{PINProvider: PINProviderFunc(testPIN)})
			key := selectedKey(t, store, context.Background())
			for _, hash := range []crypto.Hash{crypto.SHA256, crypto.SHA384, crypto.SHA512} {
				digest := make([]byte, hash.Size())
				sig, err := key.Sign(rand.Reader, digest, hash)
				if err != nil {
					t.Fatal(err)
				}
				switch public := private.Public().(type) {
				case *rsa.PublicKey:
					if err := rsa.VerifyPKCS1v15(public, hash, digest, sig); err != nil {
						t.Fatal(err)
					}
				case *ecdsa.PublicKey:
					if !ecdsa.VerifyASN1(public, digest, sig) {
						t.Fatal("DER EC inválido")
					}
				}
			}
			if _, err := key.Sign(rand.Reader, make([]byte, 32), &rsa.PSSOptions{Hash: crypto.SHA256}); !errors.Is(err, ErrMechanismUnsupported) {
				t.Fatal(err)
			}
			if _, err := key.Sign(rand.Reader, make([]byte, 20), crypto.SHA1); !errors.Is(err, ErrMechanismUnsupported) {
				t.Fatal(err)
			}
			if _, err := key.Sign(rand.Reader, make([]byte, 31), crypto.SHA256); !errors.Is(err, ErrMechanismUnsupported) {
				t.Fatal(err)
			}
		})
	}
}

func TestIdentityBindingAndHiddenKeys(t *testing.T) {
	m := newFakeModule(t)
	m.tokens[2] = tokenInfo{label: "Token QA", serial: "sintetico-2", initialized: true, loginRequired: true}
	m.hidePrivate = true
	private := ecdsaKey(t)
	addIdentity(t, m, 1, 1, private, time.Now())
	addIdentity(t, m, 2, 1, private, time.Now())
	m.objects[2].der = append([]byte(nil), m.objects[0].der...)
	store := testStore(t, m, Options{PINProvider: PINProviderFunc(testPIN)})
	refs, err := store.List(context.Background())
	if err != nil || len(refs) != 2 {
		t.Fatal(err)
	}
	if refs[0].ID == refs[1].ID {
		t.Fatal("ID confunde tokens")
	}
	for _, ref := range refs {
		if ref.HasSigningKey {
			t.Fatal("clave oculta anunciada como demostrada")
		}
		if !ref.SigningKeyNeedsUnlock {
			t.Fatal("token con login y clave no demostrada debe permitir desbloqueo explícito")
		}
	}
	for _, ref := range refs {
		key, err := store.KeyFor(context.Background(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := key.(*SigningIdentity).Sign(rand.Reader, make([]byte, 32), crypto.SHA256); err != nil {
			t.Fatal(err)
		}
	}
	forged := refs[0]
	forged.Fingerprint = "otra"
	if _, err := store.KeyFor(context.Background(), forged); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatal(err)
	}
}

func TestUnlockMetadataRequiresLoginAndUnprovenKey(t *testing.T) {
	for _, login := range []bool{false, true} {
		for _, hidden := range []bool{false, true} {
			m := newFakeModule(t)
			token := m.tokens[1]
			token.loginRequired = login
			m.tokens[1] = token
			m.hidePrivate = hidden
			addIdentity(t, m, 1, 1, ecdsaKey(t), time.Now())
			store := testStore(t, m, Options{})
			refs, err := store.List(context.Background())
			if err != nil || len(refs) != 1 {
				t.Fatal(err)
			}
			if refs[0].HasSigningKey != !hidden || refs[0].SigningKeyNeedsUnlock != (hidden && login) {
				t.Fatalf("login=%v hidden=%v ref=%+v", login, hidden, refs[0])
			}
		}
	}
}

func TestFailuresNeverProduceSignatureAndCloseSession(t *testing.T) {
	for _, name := range []string{"pin_incorrecto", "pin_bloqueado", "pin_caducado", "cancelar_pin", "proveedor_secreto", "sin_proveedor", "token_sustituido", "certificado_sustituido", "clave_duplicada", "clave_equivocada", "mecanismo_ausente", "firma_corrupta", "fallo_firma", "fallo_cierre", "cancelar_durante_firma", "cerrar_identidad"} {
		t.Run(name, func(t *testing.T) {
			m := newFakeModule(t)
			private := ecdsaKey(t)
			addIdentity(t, m, 1, 1, private, time.Now())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pin := []byte("1234")
			provider := PINProviderFunc(func(context.Context, PINRequest) ([]byte, error) { return pin, nil })
			store := testStore(t, m, Options{PINProvider: provider})
			key := selectedKey(t, store, ctx)
			want := ErrIdentityNotFound
			switch name {
			case "pin_incorrecto":
				m.loginError = ErrPINIncorrect
				want = ErrPINIncorrect
			case "pin_bloqueado":
				m.loginError = ErrPINLocked
				want = ErrPINLocked
			case "pin_caducado":
				m.loginError = ErrPINExpired
				want = ErrPINExpired
			case "cancelar_pin":
				store.options.PINProvider = PINProviderFunc(func(context.Context, PINRequest) ([]byte, error) { cancel(); return pin, nil })
				want = context.Canceled
			case "proveedor_secreto":
				store.options.PINProvider = PINProviderFunc(func(context.Context, PINRequest) ([]byte, error) { return pin, errors.New("secreto") })
				want = ErrPINUnavailable
			case "sin_proveedor":
				store.options.PINProvider = nil
				want = ErrPINUnavailable
			case "token_sustituido":
				info := m.tokens[1]
				info.serial = "otro"
				m.tokens[1] = info
			case "certificado_sustituido":
				m.objects[0].der = []byte("otro DER")
			case "clave_duplicada":
				o := m.objects[1]
				o.handle = 99
				m.objects = append(m.objects, o)
				want = ErrAmbiguousIdentity
			case "clave_equivocada":
				m.objects[1].private = ecdsaKey(t)
				want = ErrSignatureInvalid
			case "mecanismo_ausente":
				m.mechanisms = nil
				want = ErrMechanismUnsupported
			case "firma_corrupta":
				m.corruptSignature = true
				want = ErrSignatureInvalid
			case "fallo_firma":
				m.signError = ErrTokenUnavailable
				want = ErrTokenUnavailable
			case "fallo_cierre":
				m.closeError = ErrTokenUnavailable
				want = ErrTokenUnavailable
			case "cancelar_durante_firma":
				m.signHook = cancel
				want = context.Canceled
			case "cerrar_identidad":
				key.Close()
				want = ErrClosed
			}
			sig, err := key.Sign(rand.Reader, make([]byte, 32), crypto.SHA256)
			if !errors.Is(err, want) || len(sig) != 0 {
				t.Fatalf("firma=%x error=%v esperado=%v", sig, err, want)
			}
			if err != nil && bytes.Contains([]byte(err.Error()), []byte("secreto")) {
				t.Fatal("secreto en error")
			}
			if m.logins > 1 {
				t.Fatal("reintento de PIN")
			}
			if m.logins > 0 || name == "cancelar_pin" || name == "proveedor_secreto" {
				if !bytes.Equal(pin, make([]byte, len(pin))) {
					t.Fatal("PIN no zeroizado")
				}
			}
		})
	}
}

func TestContextAuthenticationAndProtectedPIN(t *testing.T) {
	for _, protected := range []bool{false, true} {
		t.Run(map[bool]string{true: "teclado", false: "pin_local"}[protected], func(t *testing.T) {
			m := newFakeModule(t)
			info := m.tokens[1]
			info.protectedAuthentication = protected
			m.tokens[1] = info
			addIdentity(t, m, 1, 1, ecdsaKey(t), time.Now())
			m.objects[1].always = true
			var requests []PINRequest
			store := testStore(t, m, Options{PINProvider: PINProviderFunc(func(_ context.Context, r PINRequest) ([]byte, error) {
				requests = append(requests, r)
				if protected {
					return nil, nil
				}
				return []byte("1234"), nil
			})})
			m.loginHook = func(pin []byte, specific bool) {
				if protected && pin != nil {
					t.Fatal("teclado protegido recibió PIN")
				}
			}
			key := selectedKey(t, store, context.Background())
			if _, err := key.Sign(rand.Reader, make([]byte, 32), crypto.SHA256); err != nil {
				t.Fatal(err)
			}
			if len(requests) != 2 || requests[0].ContextSpecific || !requests[1].ContextSpecific || requests[0].ProtectedAuthenticationPath != protected {
				t.Fatalf("peticiones=%+v", requests)
			}
			if m.logouts != 1 {
				t.Fatal("logout incorrecto")
			}
		})
	}
}

func TestCertificateExpiresDuringPIN(t *testing.T) {
	now := time.Now()
	m := newFakeModule(t)
	addIdentity(t, m, 1, 1, ecdsaKey(t), now)
	store := testStore(t, m, Options{Clock: fixedClock{now}})
	store.options.PINProvider = PINProviderFunc(func(context.Context, PINRequest) ([]byte, error) {
		store.options.Clock = fixedClock{now.Add(2 * time.Hour)}
		return []byte("1234"), nil
	})
	key := selectedKey(t, store, context.Background())
	sig, err := key.Sign(rand.Reader, make([]byte, 32), crypto.SHA256)
	if !errors.Is(err, signingpolicy.ErrCertificateUnsuitable) || len(sig) != 0 || m.signs != 0 {
		t.Fatalf("firma tras caducidad: %x %v", sig, err)
	}
}

func TestConcurrentSignAndClose(t *testing.T) {
	m := newFakeModule(t)
	addIdentity(t, m, 1, 1, ecdsaKey(t), time.Now())
	store := testStore(t, m, Options{PINProvider: PINProviderFunc(testPIN)})
	key := selectedKey(t, store, context.Background())
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := key.Sign(rand.Reader, make([]byte, 32), crypto.SHA256)
			if err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("Sign: %v", err)
			}
		}()
	}
	key.Close()
	group.Wait()
	store.Cerrar()
	store.Cerrar()
	if m.finalizes != 1 || m.destroys != 1 {
		t.Fatalf("finalizaciones=%d destrucciones=%d", m.finalizes, m.destroys)
	}
	if _, err := store.List(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestFindFailureClosesSearchAndSession(t *testing.T) {
	for _, final := range []bool{false, true} {
		m := newFakeModule(t)
		store := testStore(t, m, Options{})
		if final {
			m.finalError = ErrTokenUnavailable
		} else {
			m.findError = ErrTokenUnavailable
		}
		if _, err := store.List(context.Background()); !errors.Is(err, ErrTokenUnavailable) {
			t.Fatal(err)
		}
	}
}

func TestLegacySignRefusesAmbiguousRequest(t *testing.T) {
	m := newFakeModule(t)
	store := testStore(t, m, Options{})
	if _, err := store.KeyFor(context.Background(), domain.CertificateRef{}); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatal(err)
	}
	if _, err := store.Sign(context.Background(), store.tokenRef(1, m.tokens[1]), make([]byte, 32)); !errors.Is(err, ErrAmbiguousSigningRequest) {
		t.Fatal(err)
	}
	if m.signs != 0 || m.logins != 0 {
		t.Fatal("se firmó sin selección de identidad")
	}
}

func TestCancellationWhileAnotherOperationOwnsModule(t *testing.T) {
	m := newFakeModule(t)
	addIdentity(t, m, 1, 1, ecdsaKey(t), time.Now())
	entered, release := make(chan struct{}), make(chan struct{})
	store := testStore(t, m, Options{PINProvider: PINProviderFunc(func(context.Context, PINRequest) ([]byte, error) {
		close(entered)
		<-release
		return []byte("1234"), nil
	})})
	first := selectedKey(t, store, context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	second := selectedKey(t, store, ctx)
	finished := make(chan error, 1)
	go func() { _, err := first.Sign(rand.Reader, make([]byte, 32), crypto.SHA256); finished <- err }()
	<-entered
	cancel()
	if _, err := second.Sign(rand.Reader, make([]byte, 32), crypto.SHA256); !errors.Is(err, context.Canceled) {
		t.Errorf("espera cancelada: %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if m.signs != 1 || m.logins != 1 {
		t.Fatal("la operación cancelada alcanzó el módulo")
	}
}

func TestExistingAuthenticationIsNotLoggedOut(t *testing.T) {
	m := newFakeModule(t)
	addIdentity(t, m, 1, 1, ecdsaKey(t), time.Now())
	m.loginError = errAlreadyLoggedIn
	store := testStore(t, m, Options{PINProvider: PINProviderFunc(testPIN)})
	key := selectedKey(t, store, context.Background())
	if _, err := key.Sign(rand.Reader, make([]byte, 32), crypto.SHA256); err != nil {
		t.Fatal(err)
	}
	if m.logouts != 0 {
		t.Fatal("logout de autenticación ajena")
	}
}

type repeatedSearchModule struct{ *fakeModule }

func (m repeatedSearchModule) Find(id sessionHandle, count int) ([]objectHandle, error) {
	m.session(id)
	return []objectHandle{1}, nil
}
func TestBrokenModuleCannotLoopObjectEnumeration(t *testing.T) {
	m := newFakeModule(t)
	store := testStore(t, m, Options{})
	store.load = func(string) (module, error) { return repeatedSearchModule{m}, nil }
	if _, err := store.List(context.Background()); !errors.Is(err, ErrAmbiguousIdentity) {
		t.Fatal(err)
	}
}

func TestCheckAvailableExposesLoadFailureWithoutChangingList(t *testing.T) {
	store := New("/ruta/QA/inexistente/modulo.so")
	defer store.Cerrar()
	if err := store.CheckAvailable(context.Background()); !errors.Is(err, ErrModuloNoDisponible) {
		t.Fatalf("CheckAvailable: %v", err)
	}
	if refs, err := store.List(context.Background()); err != nil || len(refs) != 0 {
		t.Fatalf("List cambió: %v %v", refs, err)
	}
}

func TestCheckAvailableIsLazyCancellableAndIdempotent(t *testing.T) {
	m := newFakeModule(t)
	store := testStore(t, m, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CheckAvailable(ctx); !errors.Is(err, context.Canceled) || m.loads != 0 {
		t.Fatalf("cancelación: %v cargas=%d", err, m.loads)
	}
	for i := 0; i < 2; i++ {
		if err := store.CheckAvailable(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if m.loads != 1 || m.initializes != 1 || m.opens != 0 || m.logins != 0 {
		t.Fatal("CheckAvailable enumeró objetos, pidió PIN o repitió carga")
	}
	store.Cerrar()
	if err := store.CheckAvailable(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
