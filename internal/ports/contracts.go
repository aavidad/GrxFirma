// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"crypto"
	"crypto/x509"
	"time"

	"grxfirma/internal/domain"
)

// SigningKey representa una referencia opaca a una clave de firma
// gestionada por un adaptador concreto.
type SigningKey interface {
	KeyID() string
	CertificateChainDER() [][]byte
}

// TokenRef representa un token o dispositivo hardware disponible.
type TokenRef struct {
	ID    string
	Label string
}

// Evidence representa una evidencia ya saneada y lista para persistirse.
type Evidence struct {
	Type      string
	Timestamp time.Time
	Payload   []byte
}

// Event representa un evento de progreso o resultado publicable hacia el borde.
type Event struct {
	Type      string
	Timestamp time.Time
	Payload   []byte
}

// CapabilityProfile describe las capacidades observables de la plataforma actual.
type CapabilityProfile struct {
	HasSecureStorage    bool
	HasBiometricPrompt  bool
	HasSmartCardAccess  bool
	HasDocumentPicker   bool
	HasLocalServer      bool
	HasNativeMessaging  bool
	HasLegacyAfirmaURI  bool
	HasMobileDeepLink   bool
	HasLocalTLSTrust    bool
	HasPDFPreview       bool
	HasTemporaryStorage bool
}

type CertificateCatalog interface {
	// List enumera referencias sin abrir ni utilizar claves privadas. Cada
	// adaptador debe informar HasSigningKey con los metadatos no interactivos
	// de su almacén; listar nunca debe provocar una solicitud de PIN.
	List(ctx context.Context) ([]domain.CertificateRef, error)
}

type SigningKeyProvider interface {
	// KeyFor transfiere al llamador la referencia devuelta. El consumidor debe
	// invocar CloseSigningKey cuando termine, aunque el adaptador actual no
	// retenga recursos nativos.
	KeyFor(ctx context.Context, certificate domain.CertificateRef) (SigningKey, error)
}

type CertificateImporter interface {
	Import(ctx context.Context, data []byte, password string) (domain.CertificateRef, error)
}

// CertificateAccessManager es un gestor del sistema o navegador que el usuario
// puede abrir explícitamente para administrar sus certificados.
type CertificateAccessManager struct {
	ID          string
	Label       string
	Recommended bool
}

// CertificateImportTarget es un almacén persistente descubierto en la
// plataforma. Importar en él siempre requiere una acción explícita del usuario.
type CertificateImportTarget struct {
	ID          string
	Label       string
	Browser     string
	Recommended bool
}

// CertificateAccessOptions agrupa las acciones persistentes disponibles sin
// mezclararlas con el uso temporal de credenciales.
type CertificateAccessOptions struct {
	Managers         []CertificateAccessManager
	ImportTargets    []CertificateImportTarget
	DetectedBrowser  string
	PreferredManager string
	PreferredTarget  string
}

// CertificateAccess permite descubrir y ejecutar acciones explícitas sobre
// almacenes persistentes del sistema o del navegador.
type CertificateAccess interface {
	Options(ctx context.Context) (CertificateAccessOptions, error)
	OpenManager(ctx context.Context, managerID string) error
	Import(ctx context.Context, targetID string, data []byte, password string) error
}

// TemporaryCertificateStore conserva una identidad únicamente en memoria
// durante la sesión del proceso. También actúa como catálogo y proveedor de
// clave para integrarse con los casos de uso de firma existentes.
type TemporaryCertificateStore interface {
	CertificateCatalog
	SigningKeyProvider
	Load(ctx context.Context, data []byte, password string) (domain.CertificateRef, error)
	Remove(certificateID string)
	Clear()
}

type TrustAnchorProvider interface {
	Anchors(ctx context.Context) (domain.CertificateChain, error)
}

type SignerEngine interface {
	Sign(ctx context.Context, job domain.SignatureJob, key SigningKey) (domain.SignatureResult, error)
}

type VerifierEngine interface {
	Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
}

type ProtectionRecipientCatalog interface {
	Resolve(ctx context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, error)
}

type ProtectionKeyProvider interface {
	DecryptionKeys(ctx context.Context) ([]domain.ProtectionKeyMaterial, error)
}

type ProtectionRecipientExchangeStore interface {
	Export(ctx context.Context, recipientID string) (domain.ProtectionRecipient, []byte, error)
	Import(ctx context.Context, data []byte) (domain.ProtectionRecipient, error)
}

type ProtectorEngine interface {
	Protect(ctx context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient) (domain.ProtectedPayload, error)
	Unprotect(ctx context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error)
}

// SignedProtectorEngine crea contenedores de protección que además integran
// una firma del remitente, manteniendo este flujo separado del caso de uso de
// protección simple.
type SignedProtectorEngine interface {
	ProtectAndSign(ctx context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient, key SigningKey) (domain.ProtectedPayload, error)
}

type TimestampAuthority interface {
	RequestTimestamp(ctx context.Context, hash []byte, hashAlgo crypto.Hash) ([]byte, error)
}

type RevocationEvidence struct {
	OCSPResponses [][]byte
	CRLs          [][]byte
}

type RevocationProvider interface {
	Fetch(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (RevocationEvidence, error)
}

type ResultTransport interface {
	Upload(ctx context.Context, session domain.ExchangeSession, data []byte) error
	Retrieve(ctx context.Context, session domain.ExchangeSession) ([]byte, error)
	SendWait(ctx context.Context, session domain.ExchangeSession) error
	Cancel(ctx context.Context, session domain.ExchangeSession) error
}

type DirectoryTreeEntry struct {
	Path         string
	RelativePath string
}

type DirectoryTreeReader interface {
	ListFiles(ctx context.Context, rootPath string, recursive bool) ([]DirectoryTreeEntry, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
}

type DirectoryHashManifestCodec interface {
	EncodeManifest(ctx context.Context, manifest domain.DirectoryHashManifest, format domain.DirectoryHashManifestFormat) ([]byte, error)
	DecodeManifest(ctx context.Context, data []byte, hint string) (domain.DirectoryHashManifest, error)
}

type DirectoryHashReportCodec interface {
	EncodeReport(ctx context.Context, report domain.DirectoryHashCheckReport) ([]byte, error)
}

type TrustPolicy interface {
	Evaluate(ctx context.Context, origin string) (domain.TrustDecision, error)
	Allow(ctx context.Context, origin string) error
	Deny(ctx context.Context, origin string) error
	Remove(ctx context.Context, origin string) error
}

type UserApproval interface {
	Request(ctx context.Context, message string) (bool, error)
}

type EvidenceLogger interface {
	Log(ctx context.Context, evidence Evidence) error
}

type TempFileStore interface {
	Write(ctx context.Context, data []byte) (path string, err error)
	Delete(ctx context.Context, path string) error
}

type DocumentPicker interface {
	Pick(ctx context.Context) (domain.Document, error)
}

// DocumentFilter restringe los ficheros que ofrece un selector documental.
// Extensions contiene extensiones sin punto y en minúsculas; vacío significa
// que se admite cualquier fichero.
type DocumentFilter struct {
	Extensions []string
}

// FilteredDocumentPicker es un DocumentPicker que además sabe filtrar por
// tipo de fichero (por ejemplo, solo PDF cuando la web pide PAdES).
type FilteredDocumentPicker interface {
	DocumentPicker
	PickFiltered(ctx context.Context, filter DocumentFilter) (domain.Document, error)
}

type Clock interface {
	Now() time.Time
}

type DesktopNotification interface {
	Notify(ctx context.Context, title, body string) error
}

type MobilePushNotification interface {
	Push(ctx context.Context, title, body string) error
}

type SecureStorage interface {
	Store(ctx context.Context, key string, value []byte) error
	Load(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

type SmartCardAccess interface {
	Enumerate(ctx context.Context) ([]TokenRef, error)
	Sign(ctx context.Context, token TokenRef, digest []byte) ([]byte, error)
}

type BiometricPrompt interface {
	Authenticate(ctx context.Context, reason string) (bool, error)
}

type CapabilityProfileProvider interface {
	Profile(ctx context.Context) (CapabilityProfile, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}

type OperationMetrics interface {
	RecordSign(ctx context.Context, format string, status string, duration time.Duration)
	RecordCertificateSource(ctx context.Context, sourceType string)
	RecordProtocolRequest(ctx context.Context, op string, status string, duration time.Duration)
}

// Localizador traduce claves de mensaje al idioma configurado en el sistema.
// Los adaptadores de entrada lo usan para emitir mensajes de usuario en el
// idioma correcto. El dominio y la capa de aplicacion nunca lo importan.
type Localizador interface {
	T(id string, args ...any) string
}

// VisualizadorPDF renderiza paginas de un PDF como imagen PNG codificada en base64.
type VisualizadorPDF interface {
	RenderizarPagina(ctx context.Context, ruta string, pagina int) (b64 string, ancho, alto float64, totalPaginas int, err error)
}

// EstadoServicio describe el estado observable del servicio de usuario de GrxFirma.
type EstadoServicio struct {
	Instalado  bool
	Activo     bool
	Plataforma string
	Metodo     string
}

// GestorServicio gestiona el ciclo de vida del servicio de sistema de GrxFirma.
type GestorServicio interface {
	Estado(ctx context.Context) (EstadoServicio, error)
	Instalar(ctx context.Context, socketPath string) error
	Desinstalar(ctx context.Context) error
	Iniciar(ctx context.Context) error
	Detener(ctx context.Context) error
}

// ConfiguracionUsuario persiste las preferencias del usuario en el sistema de archivos local.
type ConfiguracionUsuario interface {
	Cargar(ctx context.Context) (map[string]any, error)
	Guardar(ctx context.Context, datos map[string]any) error
}
