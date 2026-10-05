// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/inbound/mobile/androidintent"
	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/adapters/outbound/common/revocationclient"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"software.sslmate.com/src/go-pkcs12"
)

const (
	mobileContractVersion     = 2
	maxCertificateChainLength = 16
)

var (
	errMobilePKCS12Decode               = errors.New("la identidad PKCS#12 no se puede decodificar")
	errMobileCertificateNotCurrent      = errors.New("el certificado no esta vigente")
	errMobileSigningIdentityUnsupported = errors.New("la identidad no es apta para firma mobile")
)

// NewAndroidFacade monta una sesion criptografica Android completamente
// operativa. La identidad importada se conserva solo en memoria hasta que se
// reemplace, se llame a ClearSession o termine el proceso.
func NewAndroidFacade(filesDir, noBackupFilesDir string) (*Facade, error) {
	if err := validateRequiredDirectory("filesDir", filesDir); err != nil {
		return nil, err
	}
	if err := validateRequiredDirectory("noBackupFilesDir", noBackupFilesDir); err != nil {
		return nil, err
	}
	if filepath.Clean(filesDir) == filepath.Clean(noBackupFilesDir) {
		return nil, newFacadeError("filesDir y noBackupFilesDir deben ser directorios distintos")
	}
	temporaryDirectory, err := preparePrivateTemporaryDirectory(noBackupFilesDir)
	if err != nil {
		return nil, err
	}
	return newPlatformFacade("android", true, temporaryDirectory)
}

// NewIOSFacade monta la misma sesion criptografica para una UI nativa iOS.
// appGroupDir y keychainAccessGroup se validan como reservas de integracion,
// pero esta version no persiste la identidad ni afirma usar Keychain.
func NewIOSFacade(applicationSupportDir, appGroupDir, keychainAccessGroup string) (*Facade, error) {
	if err := prepareApplicationSupportDirectory(applicationSupportDir); err != nil {
		return nil, err
	}
	if err := validateOptionalDirectory("appGroupDir", appGroupDir); err != nil {
		return nil, err
	}
	if err := validateBoundedText("keychainAccessGroup", keychainAccessGroup, 255, true); err != nil {
		return nil, err
	}
	temporaryDirectory, err := preparePrivateTemporaryDirectory(applicationSupportDir)
	if err != nil {
		return nil, err
	}
	return newPlatformFacade("ios", false, temporaryDirectory)
}

func newPlatformFacade(platform string, includeAndroidIntent bool, temporaryDirectory string) (*Facade, error) {
	store := newSessionIdentityStore()
	approval := nativeExplicitApproval{}
	signEngine := &mobileSignerEngine{
		delegate:           desktopsigner.NuevoMotorFirmaGo(nil).WithRevocationProvider(revocationclient.New()),
		temporaryDirectory: temporaryDirectory,
	}
	verifyEngine := newOfflineMultiVerifier()

	signUseCase := application.NuevoSignDocumentUseCase(
		store,
		store,
		signEngine,
		approval,
		nil,
		nil,
	)
	verifyUseCase := application.NuevoVerifySignatureUseCase(store, verifyEngine, nil)
	selectUseCase := application.NuevoSelectCertificateUseCase(store, approval, nil, nil)
	importUseCase := application.NuevoImportCertificateUseCase(store, nil, nil)
	profileUseCase := application.NuevoResolvePlatformProfileUseCase(
		mobileCapabilityProvider{},
		nil,
		nil,
	)

	facade := newFacade(signUseCase, verifyUseCase, selectUseCase)
	facade.importService = importUseCase
	facade.batchService = application.NuevoProcessBatchUseCase(store, store, signEngine, approval, nil, nil)
	facade.profileService = profileUseCase
	facade.session = store
	facade.temporaryDirectory = temporaryDirectory
	facade.revocationMode = "embedded_evidence_only"
	if includeAndroidIntent {
		facade.androidIntentAdapter = androidintent.New(nil)
	}

	contract, err := buildMobileContract(platform, includeAndroidIntent)
	if err != nil {
		return nil, newFacadeError("no se pudo construir el contrato mobile")
	}
	facade.contractJSON = contract
	return facade, nil
}

// MobileContractJSON describe exclusivamente capacidades configuradas en esta
// instancia. No incluye rutas, identificadores de certificado ni secretos.
func (f *Facade) MobileContractJSON() string {
	if f == nil || f.contractJSON == "" {
		return `{"contract_version":0,"platform":"unknown","services":{}}`
	}
	return f.contractJSON
}

// ClearSession elimina la referencia a la identidad importada y aplica una
// limpieza de mejor esfuerzo al material privado soportado.
func (f *Facade) ClearSession() {
	if f == nil || f.session == nil {
		return
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	f.session.clear()
	cleanPrivateTemporaryDirectory(f.temporaryDirectory)
}

// El motor PAdES compartido usa os.MkdirTemp(""). Android resuelve esa ruta
// como /data/local/tmp, que no es escribible por aplicaciones. Este adaptador
// limita temporalmente TMPDIR al directorio privado recibido de la UI nativa.
// El mutex cubre todas las fachadas del proceso y evita cambios concurrentes
// del entorno durante una firma PAdES.
type mobileSignerEngine struct {
	delegate           ports.SignerEngine
	temporaryDirectory string
}

var mobileTemporaryDirectoryMu sync.Mutex

func (m *mobileSignerEngine) Sign(
	ctx context.Context,
	job domain.SignatureJob,
	key ports.SigningKey,
) (domain.SignatureResult, error) {
	if m == nil || m.delegate == nil {
		return domain.SignatureResult{}, errors.New("motor de firma mobile no configurado")
	}
	if job.Format != domain.FormatPAdES {
		return m.delegate.Sign(ctx, job, key)
	}
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}

	mobileTemporaryDirectoryMu.Lock()
	defer mobileTemporaryDirectoryMu.Unlock()
	if err := ensurePrivateTemporaryDirectory(m.temporaryDirectory); err != nil {
		return domain.SignatureResult{}, err
	}

	previous, wasSet := os.LookupEnv("TMPDIR")
	if err := os.Setenv("TMPDIR", m.temporaryDirectory); err != nil {
		return domain.SignatureResult{}, errors.New("no se pudo preparar el almacenamiento temporal de firma")
	}
	result, signErr := m.delegate.Sign(ctx, job, key)
	restoreErr := restoreEnvironment("TMPDIR", previous, wasSet)
	if signErr != nil {
		zeroBytes(result.Data)
		return domain.SignatureResult{}, signErr
	}
	if restoreErr != nil {
		zeroBytes(result.Data)
		return domain.SignatureResult{}, errors.New("no se pudo restaurar el entorno temporal de firma")
	}
	return result, nil
}

func preparePrivateTemporaryDirectory(baseDirectory string) (string, error) {
	temporaryDirectory := filepath.Join(baseDirectory, ".grxfirma-tmp")
	mobileTemporaryDirectoryMu.Lock()
	defer mobileTemporaryDirectoryMu.Unlock()
	if err := ensurePrivateTemporaryDirectory(temporaryDirectory); err != nil {
		return "", err
	}
	cleanPrivateTemporaryDirectoryUnlocked(temporaryDirectory)
	return temporaryDirectory, nil
}

func ensurePrivateTemporaryDirectory(directory string) error {
	if directory == "" {
		return errors.New("directorio temporal mobile no configurado")
	}
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return errors.New("no se pudo crear el directorio temporal privado")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("el directorio temporal privado no es seguro")
	}
	// #nosec G302 -- this is a directory: owner execute is required for
	// traversal, while group/other permissions remain disabled.
	if err := os.Chmod(directory, 0o700); err != nil {
		return errors.New("no se pudo proteger el directorio temporal privado")
	}
	return nil
}

func cleanPrivateTemporaryDirectory(directory string) {
	if directory == "" {
		return
	}
	mobileTemporaryDirectoryMu.Lock()
	defer mobileTemporaryDirectoryMu.Unlock()
	cleanPrivateTemporaryDirectoryUnlocked(directory)
}

func cleanPrivateTemporaryDirectoryUnlocked(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(directory, entry.Name()))
	}
}

func restoreEnvironment(name, previous string, wasSet bool) error {
	if wasSet {
		return os.Setenv(name, previous)
	}
	return os.Unsetenv(name)
}

type mobileContract struct {
	EngineVersion   string                 `json:"engine_version"`
	ContractVersion int                    `json:"contract_version"`
	Platform        string                 `json:"platform"`
	Services        map[string]bool        `json:"services"`
	IdentityStore   mobileIdentityContract `json:"identity_store"`
	Approval        string                 `json:"approval"`
	Signing         mobileSigningContract  `json:"signing"`
	Verification    mobileVerifyContract   `json:"verification"`
	Limits          mobileLimitsContract   `json:"limits"`
	Protection      mobileProtectContract  `json:"protection"`
	Hash            mobileHashContract     `json:"hash"`
}

type mobileProtectContract struct {
	Containers    []string `json:"containers"`
	MaxRecipients int      `json:"max_recipients"`
	Decryption    []string `json:"decryption"`
}

type mobileHashContract struct {
	Algorithms []string `json:"algorithms"`
	Formats    []string `json:"formats"`
}

type mobileIdentityContract struct {
	Mode          string `json:"mode"`
	Persistent    bool   `json:"persistent"`
	MaxIdentities int    `json:"max_identities"`
}

type mobileVerifyContract struct {
	CryptographicIntegrity bool   `json:"cryptographic_integrity"`
	SystemTrustAnchors     bool   `json:"system_trust_anchors"`
	Revocation             string `json:"revocation"`
}

type mobileSigningContract struct {
	ProfilesByFormat map[string][]string `json:"profiles_by_format"`
	ActionsByFormat  map[string][]string `json:"actions_by_format"`
	TSAURLSchemes    []string            `json:"tsa_url_schemes"`
	Actions          []string            `json:"actions"`
	KeyTypesByFormat map[string][]string `json:"key_types_by_format"`
}

type mobileLimitsContract struct {
	DocumentBytes     int `json:"document_bytes"`
	SignedOutputBytes int `json:"signed_output_bytes"`
	CertificateBytes  int `json:"certificate_bytes"`
	PasswordBytes     int `json:"password_bytes"`
	BatchItems        int `json:"batch_items"`
	BatchInputBytes   int `json:"batch_input_bytes"`
}

func buildMobileContract(platform string, androidIntent bool) (string, error) {
	contract := mobileContract{
		EngineVersion:   engineVersion,
		ContractVersion: mobileContractVersion,
		Platform:        platform,
		Services: map[string]bool{
			"sign":               true,
			"inspect_signature":  true,
			"seal_preview":       true,
			"verify":             true,
			"select_certificate": true,
			"import_certificate": true,
			"external_signer":    true,
			"platform_profile":   true,
			"clear_session":      true,
			"android_intent":     androidIntent,
			"process_batch":      true,
			"hash":               true,
			"protect":            true,
			"unprotect":          true,
			"protect_sign":       true,
			"remote_exchange":    false,
		},
		IdentityStore: mobileIdentityContract{
			Mode:          "memory_session",
			Persistent:    false,
			MaxIdentities: 1,
		},
		Approval: "native_ui_explicit_action",
		Signing: mobileSigningContract{
			Actions:          []string{"sign", "cosign", "countersign"},
			ProfilesByFormat: map[string][]string{"CAdES": {"baseline", "t", "lt", "lta"}, "PAdES": {"baseline", "t", "lt"}, "XAdES": {"baseline", "t"}},
			ActionsByFormat:  map[string][]string{"CAdES": {"sign", "cosign", "countersign"}, "PAdES": {"sign", "cosign"}, "XAdES": {"sign", "cosign", "countersign"}},
			TSAURLSchemes:    []string{"http", "https"},
			KeyTypesByFormat: map[string][]string{
				"CAdES": {"RSA", "ECDSA"},
				"PAdES": {"RSA", "ECDSA"},
				"XAdES": {"RSA"},
			},
		},
		Verification: mobileVerifyContract{
			CryptographicIntegrity: true,
			SystemTrustAnchors:     false,
			Revocation:             "embedded_evidence_only",
		},
		Limits: mobileLimitsContract{
			DocumentBytes:     maxDocumentBytes,
			SignedOutputBytes: maxSignedDocumentBytes,
			CertificateBytes:  maxCertificateBytes,
			PasswordBytes:     maxPasswordBytes,
			BatchItems:        maxBatchItems,
			BatchInputBytes:   maxBatchInputBytes,
		},
		Protection: mobileProtectContract{
			Containers:    []string{"cms", "authenvelopeddata", "cms-encrypted", "signedandenvelopeddata"},
			MaxRecipients: maxProtectionRecipients,
			Decryption:    []string{"session_pkcs12_rsa", "transient_aes256_key"},
		},
		Hash: mobileHashContract{
			Algorithms: []string{"SHA-256", "SHA-1", "SHA-384", "SHA-512"},
			Formats:    []string{"hex", "base64", "bin"},
		},
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type nativeExplicitApproval struct{}

func (nativeExplicitApproval) Request(ctx context.Context, _ string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
}

type mobileCapabilityProvider struct{}

func (mobileCapabilityProvider) Profile(ctx context.Context) (ports.CapabilityProfile, error) {
	if err := ctx.Err(); err != nil {
		return ports.CapabilityProfile{}, err
	}
	return ports.CapabilityProfile{
		HasSecureStorage:    false,
		HasBiometricPrompt:  false,
		HasSmartCardAccess:  false,
		HasDocumentPicker:   true,
		HasLocalServer:      false,
		HasNativeMessaging:  false,
		HasLegacyAfirmaURI:  false,
		HasMobileDeepLink:   false,
		HasLocalTLSTrust:    false,
		HasPDFPreview:       false,
		HasTemporaryStorage: true,
	}, nil
}

type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("la comprobacion de revocacion en red no esta habilitada en mobile")
}

func newOfflineMultiVerifier() ports.VerifierEngine {
	client := &http.Client{Transport: offlineTransport{}, Timeout: time.Second}
	cades := commonsigner.NewCAdESVerifierWithChecker(
		commonsigner.NewRevocationCheckerWithClient(client),
	)
	return commonsigner.NewMultiVerifierWithEngines(
		cades,
		nil,
		nil,
		commonsigner.NewPAdESVerifierWithCAdES(cades),
		nil,
		nil,
		nil,
		nil,
	)
}

type sessionIdentity struct {
	reference   domain.CertificateRef
	signer      crypto.Signer
	certificate *x509.Certificate
	chain       []*x509.Certificate
}

// ExternalDigestSigner es el único acceso del núcleo a una clave no exportable.
// gomobile implementa esta interfaz en Android; nunca recibe la clave privada.
type ExternalDigestSigner interface {
	SignDigest(digest []byte, hashName string) ([]byte, error)
}

type externalRSASigner struct {
	publicKey *rsa.PublicKey
	delegate  ExternalDigestSigner
}

func (s *externalRSASigner) Public() crypto.PublicKey { return s.publicKey }

func (s *externalRSASigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if s == nil || s.delegate == nil || s.publicKey == nil || opts == nil {
		return nil, errors.New("firmador externo no disponible")
	}
	if _, pss := opts.(*rsa.PSSOptions); pss {
		return nil, errors.New("RSA-PSS no admitido por DNIe")
	}
	hash := opts.HashFunc()
	if hash != crypto.SHA256 && hash != crypto.SHA384 && hash != crypto.SHA512 {
		return nil, errors.New("resumen no admitido por DNIe")
	}
	if len(digest) != hash.Size() {
		return nil, errors.New("longitud del resumen no valida")
	}
	copyDigest := append([]byte(nil), digest...)
	defer zeroBytes(copyDigest)
	signature, err := s.delegate.SignDigest(copyDigest, hash.String())
	if err != nil {
		return nil, errors.New("el DNIe no pudo firmar el resumen")
	}
	if err := rsa.VerifyPKCS1v15(s.publicKey, hash, digest, signature); err != nil {
		zeroBytes(signature)
		return nil, errors.New("la firma del DNIe no corresponde al certificado")
	}
	return signature, nil
}

// installExternalIdentity sustituye atómicamente la identidad en memoria.
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
	s.mu.Lock()
	oldIdentity := s.identity
	s.identity = identity
	s.mu.Unlock()
	destroySessionIdentity(oldIdentity)
	return identity.reference, nil
}

type sessionIdentityStore struct {
	mu       sync.RWMutex
	identity *sessionIdentity
}

func newSessionIdentityStore() *sessionIdentityStore {
	return &sessionIdentityStore{}
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
	s.mu.Lock()
	previous := s.identity
	s.identity = identity
	s.mu.Unlock()
	destroySessionIdentity(previous)
	return identity.reference, nil
}

func (s *sessionIdentityStore) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.identity == nil {
		return []domain.CertificateRef{}, nil
	}
	return []domain.CertificateRef{s.identity.reference}, nil
}

func (s *sessionIdentityStore) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.identity == nil {
		return nil, errors.New("no hay identidad importada en la sesion")
	}
	if certificate.ID == "" || certificate.ID != s.identity.reference.ID {
		return nil, errors.New("el certificado solicitado no pertenece a la sesion")
	}
	return desktopsigner.NuevaClaveLocalConCadena(
		s.identity.signer,
		s.identity.certificate,
		s.identity.chain,
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

func (s *sessionIdentityStore) clear() {
	s.mu.Lock()
	identity := s.identity
	s.identity = nil
	s.mu.Unlock()
	destroySessionIdentity(identity)
}

func decodePKCS12Identity(data []byte, password string) (*sessionIdentity, error) {
	privateKey, certificate, chain, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, errMobilePKCS12Decode
	}
	signer, ok := privateKey.(crypto.Signer)
	if !ok || signer == nil {
		return nil, errMobileSigningIdentityUnsupported
	}
	switch signer.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey:
	default:
		destroySigner(signer)
		return nil, errMobileSigningIdentityUnsupported
	}
	if certificate == nil {
		destroySigner(signer)
		return nil, errMobileSigningIdentityUnsupported
	}
	if len(chain) > maxCertificateChainLength {
		destroySigner(signer)
		return nil, errMobileSigningIdentityUnsupported
	}
	identity := &sessionIdentity{
		reference:   certificateReference(certificate),
		signer:      signer,
		certificate: certificate,
		chain:       append([]*x509.Certificate(nil), chain...),
	}
	if err := validateImportedIdentity(identity); err != nil {
		destroySessionIdentity(identity)
		return nil, err
	}
	return identity, nil
}

func certificateReference(certificate *x509.Certificate) domain.CertificateRef {
	fingerprintSum := sha256.Sum256(certificate.Raw)
	fingerprint := hex.EncodeToString(fingerprintSum[:])
	subject := certificate.Subject.CommonName
	if subject == "" {
		subject = certificate.Subject.String()
	}
	issuer := certificate.Issuer.CommonName
	if issuer == "" {
		issuer = certificate.Issuer.String()
	}
	typeName, organization, taxID := certutil.ClasificarCertificado(certificate)
	return domain.CertificateRef{
		ID:            fingerprint,
		Subject:       subject,
		Issuer:        issuer,
		NotAfter:      certificate.NotAfter,
		Fingerprint:   fingerprint,
		HasSigningKey: true,
		Tipo:          typeName,
		Organizacion:  organization,
		NIF:           taxID,
	}
}

func validateImportedIdentity(identity *sessionIdentity) error {
	if identity == nil || identity.signer == nil || identity.certificate == nil {
		return errMobileSigningIdentityUnsupported
	}
	if err := identity.reference.Validate(); err != nil {
		return errMobileSigningIdentityUnsupported
	}
	now := time.Now()
	if now.Before(identity.certificate.NotBefore) || now.After(identity.certificate.NotAfter) {
		return errMobileCertificateNotCurrent
	}
	if identity.certificate.KeyUsage != 0 && identity.certificate.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errMobileSigningIdentityUnsupported
	}
	switch publicKey := identity.certificate.PublicKey.(type) {
	case *rsa.PublicKey:
		if publicKey == nil || publicKey.N == nil || publicKey.N.BitLen() < 2048 {
			return errMobileSigningIdentityUnsupported
		}
	case *ecdsa.PublicKey:
		if publicKey == nil || publicKey.Curve == nil || publicKey.Curve.Params().BitSize < 256 {
			return errMobileSigningIdentityUnsupported
		}
	default:
		return errMobileSigningIdentityUnsupported
	}
	certPublic, err := x509.MarshalPKIXPublicKey(identity.certificate.PublicKey)
	if err != nil {
		return errMobileSigningIdentityUnsupported
	}
	signerPublic, err := x509.MarshalPKIXPublicKey(identity.signer.Public())
	if err != nil {
		return errMobileSigningIdentityUnsupported
	}
	if !bytes.Equal(certPublic, signerPublic) {
		return errMobileSigningIdentityUnsupported
	}
	return nil
}

func destroySessionIdentity(identity *sessionIdentity) {
	if identity == nil {
		return
	}
	destroySigner(identity.signer)
	identity.signer = nil
	identity.certificate = nil
	identity.chain = nil
	identity.reference = domain.CertificateRef{}
}

func destroySigner(signer crypto.Signer) {
	switch key := signer.(type) {
	case ed25519.PrivateKey:
		zeroBytes(key)
	case *ed25519.PrivateKey:
		if key != nil {
			zeroBytes(*key)
		}
	case *ecdsa.PrivateKey:
		if key == nil {
			return
		}
		//lint:ignore SA1019 Invalidar el escalar es la limpieza terminal de la clave.
		privateScalar := key.D
		zeroBigInt(privateScalar)
	case *rsa.PrivateKey:
		if key == nil {
			return
		}
		if key.D != nil {
			key.D.SetInt64(0)
		}
		for _, prime := range key.Primes {
			if prime != nil {
				prime.SetInt64(0)
			}
		}
		zeroBigInt(key.Precomputed.Dp)
		zeroBigInt(key.Precomputed.Dq)
		zeroBigInt(key.Precomputed.Qinv)
		//lint:ignore SA1019 Se limpian tambien componentes CRT multiprimo heredados.
		deprecatedCRTValues := key.Precomputed.CRTValues
		for i := range deprecatedCRTValues {
			zeroBigInt(deprecatedCRTValues[i].Exp)
			zeroBigInt(deprecatedCRTValues[i].Coeff)
			zeroBigInt(deprecatedCRTValues[i].R)
		}
	}
}

func zeroBigInt(value *big.Int) {
	if value != nil {
		value.SetInt64(0)
	}
}

func validateRequiredDirectory(field, value string) error {
	if err := validateBoundedText(field, value, 4096, false); err != nil {
		return err
	}
	if !filepath.IsAbs(value) || filepath.Clean(value) == string(filepath.Separator) {
		return newFacadeError(field + " debe ser una ruta absoluta de aplicacion")
	}
	info, err := os.Lstat(value)
	if err != nil {
		return newFacadeError(field + " no es accesible")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return newFacadeError(field + " debe ser un directorio real de aplicacion")
	}
	return nil
}

func validateOptionalDirectory(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return validateRequiredDirectory(field, value)
}

func prepareApplicationSupportDirectory(value string) error {
	const field = "applicationSupportDir"
	if err := validateBoundedText(field, value, 4096, false); err != nil {
		return err
	}
	if !filepath.IsAbs(value) || filepath.Clean(value) == string(filepath.Separator) {
		return newFacadeError(field + " debe ser una ruta absoluta de aplicacion")
	}
	if _, err := os.Lstat(value); errors.Is(err, os.ErrNotExist) {
		parent := filepath.Dir(value)
		parentInfo, parentErr := os.Lstat(parent)
		if parentErr != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
			return newFacadeError(field + " no tiene un directorio padre seguro")
		}
		if err := os.Mkdir(value, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return newFacadeError(field + " no se pudo crear")
		}
	}
	if err := validateRequiredDirectory(field, value); err != nil {
		return err
	}
	// #nosec G302 -- applicationSupportDir is a directory and therefore needs
	// owner execute; 0700 grants no access to group or other users.
	if err := os.Chmod(value, 0o700); err != nil {
		return newFacadeError(field + " no se pudo proteger")
	}
	return nil
}
