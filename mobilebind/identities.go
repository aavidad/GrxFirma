// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"sync"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// La sesión puede tener abiertas a la vez varias identidades: certificados
// PKCS#12 importados y, como mucho, un DNIe. Todas viven solo en memoria.
const maxSessionIdentities = 8

// mobileSessionFullKey es la clave cerrada que traduce Android cuando la
// sesión ya tiene el máximo de certificados abiertos.
const mobileSessionFullKey = "session.full"

var errMobileSessionFull = errors.New("la sesion ya tiene el maximo de identidades")

type sessionIdentityStore struct {
	mu         sync.RWMutex
	identities []*sessionIdentity
}

func newSessionIdentityStore() *sessionIdentityStore {
	return &sessionIdentityStore{}
}

// identitySnapshot copia lo público de una identidad. Nunca lleva la clave.
type identitySnapshot struct {
	id          string
	certificate *x509.Certificate
	chain       []*x509.Certificate
	external    bool
}

func isExternalIdentity(identity *sessionIdentity) bool {
	if identity == nil {
		return false
	}
	_, external := identity.signer.(*externalRSASigner)
	return external
}

// add incorpora una identidad nueva. Si ya estaba abierta (misma huella) la
// sustituye; con replaceExternal también sustituye el DNIe anterior, porque
// solo hay una lectura NFC a la vez. Las identidades sustituidas se destruyen
// fuera del cerrojo.
func (s *sessionIdentityStore) add(identity *sessionIdentity, replaceExternal bool) error {
	if s == nil || identity == nil {
		return errMobileSigningIdentityUnsupported
	}
	s.mu.Lock()
	kept := make([]*sessionIdentity, 0, len(s.identities)+1)
	var removed []*sessionIdentity
	for _, current := range s.identities {
		if current.reference.ID == identity.reference.ID || (replaceExternal && isExternalIdentity(current)) {
			removed = append(removed, current)
			continue
		}
		kept = append(kept, current)
	}
	if len(kept) >= maxSessionIdentities {
		s.mu.Unlock()
		return errMobileSessionFull
	}
	s.identities = append(kept, identity)
	s.mu.Unlock()
	for _, old := range removed {
		destroySessionIdentity(old)
	}
	return nil
}

// findLocked exige el cerrojo de lectura o escritura.
func (s *sessionIdentityStore) findLocked(id string) *sessionIdentity {
	if id == "" {
		return nil
	}
	for _, identity := range s.identities {
		if identity != nil && identity.reference.ID == id {
			return identity
		}
	}
	return nil
}

// resolveLocked busca por identificador. Sin identificador solo responde si
// hay exactamente una identidad: con varias, la app debe decir cuál usa.
func (s *sessionIdentityStore) resolveLocked(id string) *sessionIdentity {
	if id != "" {
		return s.findLocked(id)
	}
	if len(s.identities) == 1 {
		return s.identities[0]
	}
	return nil
}

func (s *sessionIdentityStore) count() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.identities)
}

func (s *sessionIdentityStore) has(id string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.findLocked(id) != nil
}

func (s *sessionIdentityStore) reference(id string) (domain.CertificateRef, bool) {
	if s == nil {
		return domain.CertificateRef{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity := s.findLocked(id)
	if identity == nil {
		return domain.CertificateRef{}, false
	}
	return identity.reference, true
}

// installExternalIdentity añade el DNIe a la sesión y sustituye el anterior.
func (s *sessionIdentityStore) installExternalIdentity(leafDER []byte, chainDER [][]byte, delegate ExternalDigestSigner) (domain.CertificateRef, error) {
	if delegate == nil || len(leafDER) == 0 || len(leafDER) > maxCertificateBytes || len(chainDER) > maxCertificateChainLength {
		return domain.CertificateRef{}, errMobileSigningIdentityUnsupported
	}
	cert, err := x509.ParseCertificate(append([]byte(nil), leafDER...))
	if err != nil {
		return domain.CertificateRef{}, errMobileSigningIdentityUnsupported
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || publicKey.N == nil || publicKey.N.BitLen() < 2048 {
		return domain.CertificateRef{}, errMobileSigningIdentityUnsupported
	}
	chain := make([]*x509.Certificate, 0, len(chainDER))
	for _, der := range chainDER {
		if len(der) == 0 || len(der) > maxCertificateBytes {
			return domain.CertificateRef{}, errMobileSigningIdentityUnsupported
		}
		issuer, err := x509.ParseCertificate(append([]byte(nil), der...))
		if err != nil || !issuer.IsCA {
			return domain.CertificateRef{}, errMobileSigningIdentityUnsupported
		}
		chain = append(chain, issuer)
	}
	previousCertificate := cert
	for _, issuer := range chain {
		if err := previousCertificate.CheckSignatureFrom(issuer); err != nil {
			return domain.CertificateRef{}, errMobileSigningIdentityUnsupported
		}
		previousCertificate = issuer
	}
	identity := &sessionIdentity{reference: certificateReference(cert), signer: &externalRSASigner{publicKey, delegate}, certificate: cert, chain: chain}
	if err := validateImportedIdentity(identity); err != nil {
		return domain.CertificateRef{}, err
	}
	if err := s.add(identity, true); err != nil {
		destroySessionIdentity(identity)
		return domain.CertificateRef{}, err
	}
	return identity.reference, nil
}

func (s *sessionIdentityStore) Import(ctx context.Context, data []byte, password string) (domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.CertificateRef{}, err
	}
	if len(data) == 0 || len(data) > maxCertificateBytes {
		return domain.CertificateRef{}, errors.New("tamano de certificado no permitido")
	}
	identity, err := decodePKCS12Identity(data, password)
	if err != nil {
		return domain.CertificateRef{}, err
	}
	reference := identity.reference
	if err := s.add(identity, false); err != nil {
		destroySessionIdentity(identity)
		return domain.CertificateRef{}, err
	}
	return reference, nil
}

func (s *sessionIdentityStore) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	refs := make([]domain.CertificateRef, 0, len(s.identities))
	for _, identity := range s.identities {
		refs = append(refs, identity.reference)
	}
	return refs, nil
}

func (s *sessionIdentityStore) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.identities) == 0 {
		return nil, errors.New("no hay identidad importada en la sesion")
	}
	identity := s.findLocked(certificate.ID)
	if identity == nil {
		return nil, errors.New("el certificado solicitado no pertenece a la sesion")
	}
	return desktopsigner.NuevaClaveLocalConCadena(
		identity.signer,
		identity.certificate,
		identity.chain,
	), nil
}

func (s *sessionIdentityStore) Anchors(ctx context.Context) (domain.CertificateChain, error) {
	if err := ctx.Err(); err != nil {
		return domain.CertificateChain{}, err
	}
	// Importar una identidad no la convierte en ancla de confianza. Los motores
	// verifican integridad y evidencias embebidas, y reportan confianza unknown.
	return domain.CertificateChain{}, nil
}

// remove cierra una identidad y destruye su material. Devuelve cuántas quedan.
func (s *sessionIdentityStore) remove(id string) (int, bool) {
	if s == nil || id == "" {
		return 0, false
	}
	s.mu.Lock()
	var removed *sessionIdentity
	kept := make([]*sessionIdentity, 0, len(s.identities))
	for _, identity := range s.identities {
		if removed == nil && identity.reference.ID == id {
			removed = identity
			continue
		}
		kept = append(kept, identity)
	}
	if removed != nil {
		s.identities = kept
	}
	remaining := len(s.identities)
	s.mu.Unlock()
	destroySessionIdentity(removed)
	return remaining, removed != nil
}

func (s *sessionIdentityStore) clear() {
	s.mu.Lock()
	identities := s.identities
	s.identities = nil
	s.mu.Unlock()
	for _, identity := range identities {
		destroySessionIdentity(identity)
	}
}

func snapshotOf(identity *sessionIdentity) identitySnapshot {
	return identitySnapshot{
		id:          identity.reference.ID,
		certificate: identity.certificate,
		chain:       append([]*x509.Certificate(nil), identity.chain...),
		external:    isExternalIdentity(identity),
	}
}

// snapshots devuelve lo público de todas las identidades, en orden de apertura.
func (s *sessionIdentityStore) snapshots() []identitySnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]identitySnapshot, 0, len(s.identities))
	for _, identity := range s.identities {
		if identity != nil && identity.certificate != nil {
			out = append(out, snapshotOf(identity))
		}
	}
	return out
}

func (s *sessionIdentityStore) snapshot(id string) (identitySnapshot, bool) {
	if s == nil {
		return identitySnapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity := s.findLocked(id)
	if identity == nil || identity.certificate == nil {
		return identitySnapshot{}, false
	}
	return snapshotOf(identity), true
}

// certificateDER devuelve el certificado público de la identidad indicada
// (o de la única abierta si no se indica ninguna).
func (s *sessionIdentityStore) certificateDER(id string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity := s.resolveLocked(id)
	if identity == nil || identity.certificate == nil {
		return nil, false
	}
	return append([]byte(nil), identity.certificate.Raw...), true
}

func (s *sessionIdentityStore) hasSigningIdentity(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity := s.resolveLocked(id)
	return identity != nil && identity.signer != nil
}

// isRSA responde por una identidad abierta. Si no existe devuelve true para
// que el caso de uso dé su error habitual de certificado ajeno.
func (s *sessionIdentityStore) isRSA(id string) bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity := s.findLocked(id)
	if identity == nil || identity.certificate == nil {
		return true
	}
	_, ok := identity.certificate.PublicKey.(*rsa.PublicKey)
	return ok
}

// mobileSessionDecryptionKeys entrega copias PKCS#8 transitorias de las claves
// RSA importadas; el caso de uso elige la del destinatario y las borra al
// terminar. Un DNIe no expone descifrado y, por tanto, no aporta claves.
type mobileSessionDecryptionKeys struct {
	store *sessionIdentityStore
}

func (k mobileSessionDecryptionKeys) DecryptionKeys(ctx context.Context) ([]domain.ProtectionKeyMaterial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k.store == nil {
		return nil, nil
	}
	k.store.mu.RLock()
	defer k.store.mu.RUnlock()
	var keys []domain.ProtectionKeyMaterial
	for _, identity := range k.store.identities {
		if identity == nil || identity.certificate == nil {
			continue
		}
		private, ok := identity.signer.(*rsa.PrivateKey)
		if !ok || private == nil {
			continue
		}
		pkcs8, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			continue
		}
		keys = append(keys, domain.ProtectionKeyMaterial{
			RecipientID:               identity.reference.ID,
			RSAOAEP256PrivateKeyPKCS8: pkcs8,
			CertificateDER:            append([]byte(nil), identity.certificate.Raw...),
		})
	}
	return keys, nil
}

type removeIdentityResponse struct {
	Removed   bool `json:"removed"`
	Remaining int  `json:"remaining"`
}

// RemoveSessionIdentityJSON cierra uno de los certificados abiertos y destruye
// su material en memoria. ClearSession sigue cerrándolos todos.
func (f *Facade) RemoveSessionIdentityJSON(payload string) (string, error) {
	if f == nil || f.session == nil {
		return "", errNoConfigurado("cierre de certificado")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req certificateRequest
	if err := decodeJSONStrict(payload, maxOnlineJSONBytes, "cierre de certificado", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, false); err != nil {
		return "", err
	}
	remaining, removed := f.session.remove(req.CertificateID)
	if !removed {
		return "", newFacadeError("certificado de sesión no disponible")
	}
	if remaining == 0 {
		cleanPrivateTemporaryDirectory(f.temporaryDirectory)
	}
	return marshal(removeIdentityResponse{Removed: true, Remaining: remaining})
}
