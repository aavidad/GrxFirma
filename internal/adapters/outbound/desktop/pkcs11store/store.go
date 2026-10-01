// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pkcs11store proporciona la biblioteca PKCS#11 del auxiliar aislado.
// No debe cargar drivers en el proceso principal (ADR-002). Los constructores
// no cargan bibliotecas ni consultan dispositivos.
package pkcs11store

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

// PINRequest solo se entrega al proveedor local inyectado por el auxiliar.
// No contiene PIN ni configuración procedente del documento.
type PINRequest struct {
	Token                       ports.TokenRef
	ProtectedAuthenticationPath bool
	ContextSpecific             bool
}

// PINProvider transfiere la propiedad del buffer: se sobrescribe inmediatamente
// después de Login, también si hay error. Para teclado protegido confirma la
// interacción devolviendo un buffer vacío.
type PINProvider interface {
	RequestPIN(context.Context, PINRequest) ([]byte, error)
}
type PINProviderFunc func(context.Context, PINRequest) ([]byte, error)

func (f PINProviderFunc) RequestPIN(ctx context.Context, request PINRequest) ([]byte, error) {
	if f == nil {
		return nil, ErrPINUnavailable
	}
	return f(ctx, request)
}

type Options struct {
	PINProvider PINProvider
	Clock       ports.Clock
}

// Almacen serializa las operaciones nativas. Una llamada C bloqueada requiere
// terminar el auxiliar; context se comprueba alrededor de las fronteras nativas.
// Cerrar es definitivo e idempotente.
type Almacen struct {
	modulePath string
	options    Options
	gate       chan struct{}
	driver     module
	load       func(string) (module, error)
	initErr    error
	closed     bool
}

func New(path string) *Almacen { return NewWithOptions(path, Options{}) }
func NewWithOptions(path string, options Options) *Almacen {
	a := &Almacen{modulePath: path, options: options, gate: make(chan struct{}, 1), load: loadNativeModule}
	a.gate <- struct{}{}
	return a
}
func NewAutodetect() *Almacen { return New(autodetectarModulo()) }

func autodetectarModulo() string {
	var candidates []string
	switch runtime.GOOS {
	case "linux":
		candidates = []string{"/usr/lib/x86_64-linux-gnu/opensc-pkcs11.so", "/usr/lib/pkcs11/opensc-pkcs11.so", "/usr/lib/opensc-pkcs11.so"}
	case "darwin":
		candidates = []string{"/Library/OpenSC/lib/opensc-pkcs11.so", "/usr/local/lib/opensc-pkcs11.so"}
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

func (a *Almacen) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.gate:
		if err := ctx.Err(); err != nil {
			a.release()
			return err
		}
		return nil
	}
}
func (a *Almacen) release() { a.gate <- struct{}{} }
func (a *Almacen) initialize() error {
	if a.closed {
		return ErrClosed
	}
	if a.driver != nil {
		return nil
	}
	if a.initErr != nil {
		return a.initErr
	}
	driver, err := a.load(a.modulePath)
	if err == nil {
		err = driver.Initialize()
		if err != nil {
			driver.Destroy()
		}
	}
	if err != nil {
		a.initErr = err
		return err
	}
	a.driver = driver
	return nil
}
func (a *Almacen) now() time.Time {
	if a.options.Clock != nil {
		return a.options.Clock.Now()
	}
	return time.Now()
}

// CheckAvailable carga e inicializa el módulo de forma explícita y devuelve
// cualquier error. El auxiliar debe usarlo para distinguir un driver ausente
// de un token sin certificados. List conserva su degradación histórica a vacío.
// Solo debe invocarse dentro del auxiliar, después del hardening.
func (a *Almacen) CheckAvailable(ctx context.Context) error {
	if err := a.acquire(ctx); err != nil {
		return err
	}
	defer a.release()
	if err := a.initialize(); err != nil {
		return err
	}
	return ctx.Err()
}

type identity struct {
	slot          uint
	token         tokenInfo
	objectID, der []byte
	ref           domain.CertificateRef
}

func fingerprint(der []byte) string { sum := sha256.Sum256(der); return hex.EncodeToString(sum[:]) }
func (a *Almacen) identityID(slot uint, token tokenInfo, id []byte, fp string) string {
	data, _ := json.Marshal([]any{filepath.Clean(a.modulePath), slot, token.manufacturer, token.model, token.serial, token.label, hex.EncodeToString(id), fp})
	return "pkcs11:v1:" + fingerprint(data)
}
func (a *Almacen) tokenRef(slot uint, token tokenInfo) ports.TokenRef {
	return ports.TokenRef{ID: a.identityID(slot, token, nil, ""), Label: token.label}
}

func (a *Almacen) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if err := a.initialize(); err != nil {
		if errors.Is(err, ErrModuloNoDisponible) {
			return nil, nil
		}
		return nil, err
	}
	identities, err := a.list(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]domain.CertificateRef, 0, len(identities))
	for _, identity := range identities {
		refs = append(refs, identity.ref)
	}
	return refs, nil
}
func (a *Almacen) list(ctx context.Context) ([]identity, error) {
	slots, err := a.driver.Slots()
	if err != nil {
		return nil, err
	}
	if len(slots) > maxSlots {
		return nil, ErrLimit
	}
	var result []identity
	for _, slot := range slots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := a.driver.TokenInfo(slot)
		if err != nil {
			return nil, err
		}
		// GetSlotList(true) también puede anunciar tokens sin inicializar
		// (SoftHSM reserva uno para aprovisionamiento). No tienen catálogo
		// utilizable; los errores de tokens inicializados siguen propagándose.
		if !token.initialized {
			continue
		}
		identities, err := a.listSlot(ctx, slot, token)
		if err != nil {
			return nil, err
		}
		result = append(result, identities...)
		if len(result) > maxObjects {
			return nil, ErrLimit
		}
	}
	return result, ctx.Err()
}
func (a *Almacen) listSlot(ctx context.Context, slot uint, token tokenInfo) (result []identity, err error) {
	session, err := a.driver.OpenSession(slot)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, a.driver.CloseSession(session))
		if err != nil {
			result = nil
		}
	}()
	objects, err := a.find(ctx, session, objectQuery{})
	if err != nil {
		return nil, err
	}
	for _, handle := range objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		der, id, err := a.driver.Certificate(session, handle)
		if err != nil {
			return nil, err
		}
		if len(der) == 0 || len(der) > maxCertificateBytes || len(id) == 0 || len(id) > maxObjectIDBytes {
			continue
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		keys, err := a.find(ctx, session, objectQuery{private: true, id: id, signing: true})
		if err != nil {
			return nil, err
		}
		fp := fingerprint(der)
		ref := domain.CertificateRef{ID: a.identityID(slot, token, id, fp), Fingerprint: fp, Subject: cert.Subject.String(), Issuer: cert.Issuer.String(), NotAfter: cert.NotAfter, HasSigningKey: len(keys) == 1, DER: cert.Raw}
		ref.SigningKeyNeedsUnlock = !ref.HasSigningKey && token.loginRequired
		result = append(result, identity{slot: slot, token: token, objectID: append([]byte(nil), id...), der: append([]byte(nil), der...), ref: ref})
	}
	return result, ctx.Err()
}
func (a *Almacen) find(ctx context.Context, session sessionHandle, query objectQuery) (result []objectHandle, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := a.driver.FindInit(session, query); err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, a.driver.FindFinal(session))
		if err != nil {
			result = nil
		}
	}()
	seen := make(map[objectHandle]bool)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := a.driver.Find(session, 16)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return result, nil
		}
		if len(page) > 16 || len(result)+len(page) > maxObjects {
			return nil, ErrLimit
		}
		for _, handle := range page {
			if seen[handle] {
				return nil, ErrAmbiguousIdentity
			}
			seen[handle] = true
			result = append(result, handle)
		}
	}
}

func (a *Almacen) Enumerate(ctx context.Context) ([]ports.TokenRef, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if err := a.initialize(); err != nil {
		if errors.Is(err, ErrModuloNoDisponible) {
			return nil, nil
		}
		return nil, err
	}
	slots, err := a.driver.Slots()
	if err != nil {
		return nil, err
	}
	if len(slots) > maxSlots {
		return nil, ErrLimit
	}
	var tokens []ports.TokenRef
	for _, slot := range slots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := a.driver.TokenInfo(slot)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, a.tokenRef(slot, info))
	}
	return tokens, ctx.Err()
}

// KeyFor resuelve identidad exacta sin solicitar PIN. El PIN se pide al ejecutar
// crypto.Signer.Sign, dentro del consentimiento gestionado por el auxiliar.
func (a *Almacen) KeyFor(ctx context.Context, reference domain.CertificateRef) (ports.SigningKey, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if err := a.initialize(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(reference.ID, "pkcs11:v1:") || reference.Fingerprint == "" {
		return nil, ErrIdentityNotFound
	}
	identities, err := a.list(ctx)
	if err != nil {
		return nil, err
	}
	var selected *identity
	for i := range identities {
		candidate := &identities[i]
		if candidate.ref.ID == reference.ID && candidate.ref.Fingerprint == reference.Fingerprint {
			if selected != nil {
				return nil, ErrAmbiguousIdentity
			}
			selected = candidate
		}
	}
	if selected == nil {
		return nil, ErrIdentityNotFound
	}
	if err := signingpolicy.ValidateCertificateDER(selected.der, a.now()); err != nil {
		return nil, err
	}
	return &SigningIdentity{store: a, identity: *selected, ctx: ctx}, nil
}

// Sign conserva el contrato, pero rechaza peticiones sin certificado ni hash.
// Use KeyFor y crypto.Signer: nunca se selecciona «la primera clave del token».
func (a *Almacen) Sign(ctx context.Context, _ ports.TokenRef, _ []byte) ([]byte, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	if err := a.initialize(); err != nil {
		return nil, err
	}
	return nil, ErrAmbiguousSigningRequest
}
func (a *Almacen) Cerrar() {
	<-a.gate
	defer a.release()
	if a.closed {
		return
	}
	a.closed = true
	if a.driver != nil {
		_ = a.driver.Finalize()
		a.driver.Destroy()
		a.driver = nil
	}
}
func trimPKCS11String(s string) string { return strings.TrimRight(s, " \x00") }

var _ ports.CertificateCatalog = (*Almacen)(nil)
var _ ports.SigningKeyProvider = (*Almacen)(nil)
var _ ports.SmartCardAccess = (*Almacen)(nil)
