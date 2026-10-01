// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// SignDocumentUseCase define el contrato minimo esperado por el endpoint de firma.
type SignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

// BatchSignUseCase define el contrato minimo esperado por el endpoint de firma por lotes.
type BatchSignUseCase interface {
	Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error)
}

// VerifySignatureUseCase define el contrato minimo esperado por el endpoint de verificacion.
type VerifySignatureUseCase interface {
	Execute(ctx context.Context, cmd application.VerifyCommand) (application.VerifyResult, error)
}

// CreateHashUseCase define el contrato minimo esperado por el endpoint de huellas.
type CreateHashUseCase interface {
	Execute(ctx context.Context, cmd application.CreateHashCommand) (application.CreateHashResult, error)
}

// CheckHashUseCase define el contrato minimo esperado por el endpoint de comprobacion de huellas.
type CheckHashUseCase interface {
	Execute(ctx context.Context, cmd application.CheckHashCommand) (application.CheckHashResult, error)
}

// CreateDirectoryHashUseCase define el contrato minimo esperado por el endpoint de hashes de directorio.
type CreateDirectoryHashUseCase interface {
	Execute(ctx context.Context, cmd application.CreateDirectoryHashManifestCommand) (application.CreateDirectoryHashManifestResult, error)
}

// CheckDirectoryHashUseCase define el contrato minimo esperado por el endpoint de comprobacion de hashes de directorio.
type CheckDirectoryHashUseCase interface {
	Execute(ctx context.Context, cmd application.CheckDirectoryHashManifestCommand) (application.CheckDirectoryHashManifestResult, error)
}

// PDFPreviewUseCase define el contrato minimo esperado por la previsualizacion
// PDF del cliente Qt.
type PDFPreviewUseCase interface {
	Ejecutar(ctx context.Context, cmd application.PdfPreviewCommand) (application.PdfPreviewResult, error)
}

// ProtectDocumentUseCase define el contrato minimo esperado por el endpoint de protección.
type ProtectDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.ProtectCommand) (application.ProtectResult, error)
}

// ProtectAndSignDocumentUseCase define el contrato mínimo esperado por el endpoint de protección firmada.
type ProtectAndSignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error)
}

// UnprotectDocumentUseCase define el contrato minimo esperado por el endpoint de desprotección.
type UnprotectDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.UnprotectCommand) (application.UnprotectResult, error)
}

// ExportProtectionRecipientUseCase define el contrato mínimo esperado para exportar destinatarios públicos fuertes.
type ExportProtectionRecipientUseCase interface {
	Execute(ctx context.Context, cmd application.ExportProtectionRecipientCommand) (application.ExportProtectionRecipientResult, error)
}

// ImportProtectionRecipientUseCase define el contrato mínimo esperado para importar destinatarios públicos fuertes.
type ImportProtectionRecipientUseCase interface {
	Execute(ctx context.Context, cmd application.ImportProtectionRecipientCommand) (application.ImportProtectionRecipientResult, error)
}

// SelectCertificateUseCase define el contrato minimo esperado por el endpoint de seleccion.
type SelectCertificateUseCase interface {
	Execute(ctx context.Context, cmd application.SelectCertificateCommand) (application.SelectCertificateResult, error)
}

// PrepareIdentityUseCase define el contrato del reto genérico sin acoplar el borde.
type PrepareIdentityUseCase interface {
	Preparar(ctx context.Context, solicitud domain.SolicitudRetoIdentidad) (domain.RetoIdentidad, error)
}

// ConfirmIdentityUseCase define el contrato trivalente de verificación genérica.
type ConfirmIdentityUseCase interface {
	Confirmar(ctx context.Context, prueba domain.PruebaIdentidad) (domain.ResultadoVerificacionIdentidad, error)
}

// CertificateCatalog expone el catálogo de certificados disponibles.
type CertificateCatalog interface {
	List(ctx context.Context) ([]domain.CertificateRef, error)
}

// SigningKeyProvider permite comprobar si un certificado tiene clave utilizable.
type SigningKeyProvider interface {
	KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error)
}

// ProtectionRecipients expone destinatarios de protección listables y resolubles.
type ProtectionRecipients interface {
	ports.ProtectionRecipientCatalog
	List(ctx context.Context) ([]domain.ProtectionRecipient, error)
}

type tlsRESTDependencies struct {
	ensureBrowserCompatibleCertificate func(
		dir string,
		prefix string,
	) (certFile, keyFile, rootCertFile, source string, err error)
	ensureManagedTrust             func(context.Context, string) error
	removeManagedTrust             func(context.Context, string) error
	managedTrustLifecycleSupported func() bool
}

func defaultTLSRESTDependencies() tlsRESTDependencies {
	return tlsRESTDependencies{
		ensureBrowserCompatibleCertificate: EnsureBrowserCompatibleLocalhostCertificate,
		ensureManagedTrust:                 localtlstrust.EnsureManagedTrusted,
		removeManagedTrust:                 localtlstrust.RemoveManagedTrusted,
		managedTrustLifecycleSupported:     localtlstrust.ManagedTrustLifecycleSupported,
	}
}

// Adaptador implementa un router HTTP minimo desacoplado del nucleo.
type Adaptador struct {
	Firmar             SignDocumentUseCase
	ProcesarLote       BatchSignUseCase
	Verificar          VerifySignatureUseCase
	CrearHash          CreateHashUseCase
	ComprobarHash      CheckHashUseCase
	CrearHashDir       CreateDirectoryHashUseCase
	ComprobarHashDir   CheckDirectoryHashUseCase
	InformeHashDir     ports.DirectoryHashReportCodec
	PreviewPDF         PDFPreviewUseCase
	Proteger           ProtectDocumentUseCase
	ProtegerFirmando   ProtectAndSignDocumentUseCase
	Desproteger        UnprotectDocumentUseCase
	ExportarProteccion ExportProtectionRecipientUseCase
	ImportarProteccion ImportProtectionRecipientUseCase
	SeleccionarCert    SelectCertificateUseCase
	PrepararIdentidad  PrepareIdentityUseCase
	ConfirmarIdentidad ConfirmIdentityUseCase
	Catalogo           CertificateCatalog
	Claves             SigningKeyProvider
	Destinatarios      ProtectionRecipients
	ProxySecrets       ports.ProxySecretStore
	Servicio           ports.GestorServicio
	Loc                ports.Localizador
	ConfigDir          string
	BearerToken        string
	SessionTTL         time.Duration
	AllowedCerts       map[string]struct{}
	// CertificateAuthEnabled solo se activa con una allowlist SHA-256
	// explícita. La confianza genérica del sistema no autoriza clientes REST.
	CertificateAuthEnabled bool
	// AllowFileSystemPaths habilita el uso de inputPath/outputPath en las peticiones
	// REST. Por defecto está desactivado: aceptar solo content_base64 por la red
	// evita que cualquier proceso local o página web lea/escriba ficheros arbitrarios
	// del usuario (CWE-22, H-01).
	AllowFileSystemPaths bool
	// AllowPDFPreviewPaths habilita exclusivamente la lectura segura de PDF para
	// /pdf/preview. No concede permisos de lectura o escritura a otros endpoints.
	AllowPDFPreviewPaths bool
	// MaxBodyBytes limita el tamaño del cuerpo de las peticiones REST (H-07, CWE-770).
	// 0 usa el valor por defecto (100 MB). Solo sobreescribir en tests.
	MaxBodyBytes       int64
	mu                 sync.Mutex
	authGeneration     uint64
	challenges         map[string]restChallenge
	sessions           map[string]restSession
	signRequests       map[string]time.Time
	signRequestsLoaded bool
	signRequestTTL     time.Duration
	tlsDeps            tlsRESTDependencies
	tlsMu              sync.Mutex
}

// New construye el adaptador REST.
func New(firmar SignDocumentUseCase, verificar VerifySignatureUseCase, seleccionar SelectCertificateUseCase) *Adaptador {
	return &Adaptador{
		Firmar:          firmar,
		Verificar:       verificar,
		SeleccionarCert: seleccionar,
		SessionTTL:      10 * time.Minute,
		AllowedCerts:    map[string]struct{}{},
		challenges:      map[string]restChallenge{},
		sessions:        map[string]restSession{},
		signRequests:    map[string]time.Time{},
		signRequestTTL:  defaultSignRequestTTL,
	}
}

// WithBatchSigner conecta el caso de uso de firma múltiple.
func (a *Adaptador) WithBatchSigner(procesarLote BatchSignUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.ProcesarLote = procesarLote
	return a
}

// WithHashes conecta los casos de uso de creación y comprobación de huellas.
func (a *Adaptador) WithHashes(crear CreateHashUseCase, comprobar CheckHashUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.CrearHash = crear
	a.ComprobarHash = comprobar
	return a
}

// WithDirectoryHashes conecta los casos de uso de hashes de directorio.
func (a *Adaptador) WithDirectoryHashes(crear CreateDirectoryHashUseCase, comprobar CheckDirectoryHashUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.CrearHashDir = crear
	a.ComprobarHashDir = comprobar
	return a
}

// WithDirectoryHashReports conecta el codec del informe de comprobación de directorios.
func (a *Adaptador) WithDirectoryHashReports(codec ports.DirectoryHashReportCodec) *Adaptador {
	if a == nil {
		return nil
	}
	a.InformeHashDir = codec
	return a
}

// WithPDFPreview conecta el caso de uso de previsualizacion PDF.
func (a *Adaptador) WithPDFPreview(preview PDFPreviewUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.PreviewPDF = preview
	return a
}

// WithProtection conecta los casos de uso y el catálogo de protección local.
func (a *Adaptador) WithProtection(proteger ProtectDocumentUseCase, desproteger UnprotectDocumentUseCase, destinatarios ProtectionRecipients) *Adaptador {
	if a == nil {
		return nil
	}
	a.Proteger = proteger
	a.Desproteger = desproteger
	a.Destinatarios = destinatarios
	return a
}

// WithProxySecretStore conecta el backend seguro de secretos de proxy.
func (a *Adaptador) WithProxySecretStore(store ports.ProxySecretStore) *Adaptador {
	if a == nil {
		return nil
	}
	a.ProxySecrets = store
	return a
}

// WithSignedProtection conecta el caso de uso de protección firmada CMS.
func (a *Adaptador) WithSignedProtection(protegerFirmando ProtectAndSignDocumentUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.ProtegerFirmando = protegerFirmando
	return a
}

// WithIntercambioProteccion conecta la exportación/importación de destinatarios públicos fuertes.
func (a *Adaptador) WithIntercambioProteccion(exportar ExportProtectionRecipientUseCase, importar ImportProtectionRecipientUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.ExportarProteccion = exportar
	a.ImportarProteccion = importar
	return a
}

// WithFileSystemPaths habilita las operaciones inputPath/outputPath en los endpoints
// REST. Por defecto están desactivadas (H-01). Actívalo solo en entornos locales de
// confianza donde el cliente sea el propio usuario del sistema.
func (a *Adaptador) WithFileSystemPaths() *Adaptador {
	if a == nil {
		return nil
	}
	a.AllowFileSystemPaths = true
	return a
}

// WithPDFPreviewPaths habilita exclusivamente las rutas locales del endpoint
// /pdf/preview. Está pensado para el backend REST autenticado de la GUI local.
func (a *Adaptador) WithPDFPreviewPaths() *Adaptador {
	if a == nil {
		return nil
	}
	a.AllowPDFPreviewPaths = true
	return a
}

// WithBearerToken activa autenticación Bearer opcional para los endpoints mutables.
func (a *Adaptador) WithBearerToken(token string) *Adaptador {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.BearerToken = strings.TrimSpace(token)
	return a
}

// WithCertificateAuth activa autenticación por certificado únicamente cuando
// se proporciona una allowlist SHA-256 no vacía. Una lista vacía la desactiva.
func (a *Adaptador) WithCertificateAuth(allowedFingerprintsCSV string, sessionTTL time.Duration) *Adaptador {
	if a == nil {
		return nil
	}
	allowed := parseAllowedFingerprints(allowedFingerprintsCSV)
	a.mu.Lock()
	defer a.mu.Unlock()
	if sessionTTL > 0 {
		a.SessionTTL = sessionTTL
	}
	a.AllowedCerts = allowed
	a.CertificateAuthEnabled = len(allowed) > 0
	a.authGeneration++
	if a.challenges == nil {
		a.challenges = map[string]restChallenge{}
	}
	if a.sessions == nil {
		a.sessions = map[string]restSession{}
	}
	// Toda reconfiguración revoca retos y sesiones emitidos bajo la política
	// anterior, incluso cuando ambas allowlists son no vacías.
	clear(a.challenges)
	clear(a.sessions)
	return a
}

// WithCertificateSources conecta catálogo y proveedor de claves para endpoints auxiliares.
func (a *Adaptador) WithCertificateSources(catalogo CertificateCatalog, claves SigningKeyProvider) *Adaptador {
	if a == nil {
		return nil
	}
	a.Catalogo = catalogo
	a.Claves = claves
	return a
}

// WithConfigDir conecta un directorio de configuración local para endpoints auxiliares.
func (a *Adaptador) WithConfigDir(configDir string) *Adaptador {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ConfigDir = strings.TrimSpace(configDir)
	a.signRequests = map[string]time.Time{}
	a.signRequestsLoaded = false
	return a
}

// WithServicio conecta un gestor de servicio para exponer acciones de servicio
// reales desde la API REST local.
func (a *Adaptador) WithServicio(servicio ports.GestorServicio) *Adaptador {
	if a == nil {
		return nil
	}
	a.Servicio = servicio
	return a
}

// WithLocalizador inyecta un localizador para textos visibles del adaptador.
func (a *Adaptador) WithLocalizador(loc ports.Localizador) *Adaptador {
	if a == nil {
		return nil
	}
	a.Loc = loc
	return a
}

func (a *Adaptador) WithMaxBodyBytes(n int64) *Adaptador {
	if a == nil {
		return nil
	}
	a.MaxBodyBytes = n
	return a
}

// WithIdentidadReforzada publica conjuntamente las dos operaciones del contrato v1.
func (a *Adaptador) WithIdentidadReforzada(preparar PrepareIdentityUseCase, confirmar ConfirmIdentityUseCase) *Adaptador {
	if a == nil {
		return nil
	}
	a.PrepararIdentidad = preparar
	a.ConfirmarIdentidad = confirmar
	return a
}

// Routes devuelve el handler HTTP del adaptador.
func (a *Adaptador) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.handleHealth)
	mux.Handle("/certificates", a.authorize(http.HandlerFunc(a.handleCertificates)))
	mux.HandleFunc("/openapi.json", a.handleOpenAPI)
	if enabled, _ := a.certificateAuthSnapshot(); enabled {
		mux.HandleFunc("/auth/challenge", a.handleAuthChallenge)
		mux.HandleFunc("/auth/verify", a.handleAuthVerify)
		mux.HandleFunc("/autenticacion/reto", a.handleAuthChallenge)
		mux.HandleFunc("/autenticacion/verificar", a.handleAuthVerify)
	}
	if a.PrepararIdentidad != nil && a.ConfirmarIdentidad != nil {
		mux.Handle("/identity/challenges", a.authorizeIdentity(http.HandlerFunc(a.handleIdentityChallenge)))
		mux.Handle("/identity/verifications", a.authorizeIdentity(http.HandlerFunc(a.handleIdentityVerification)))
	}
	mux.Handle("/settings", a.authorize(http.HandlerFunc(a.handleSettings)))
	mux.Handle("/settings/proxy/secret-store/status", a.authorize(http.HandlerFunc(a.handleProxySecretStoreStatus)))
	mux.Handle("/service/status", a.authorize(http.HandlerFunc(a.handleServiceStatus)))
	mux.Handle("/service/install", a.authorize(http.HandlerFunc(a.handleServiceInstall)))
	mux.Handle("/service/uninstall", a.authorize(http.HandlerFunc(a.handleServiceUninstall)))
	mux.Handle("/service/start", a.authorize(http.HandlerFunc(a.handleServiceStart)))
	mux.Handle("/service/stop", a.authorize(http.HandlerFunc(a.handleServiceStop)))
	mux.Handle("/pdf/preview", a.authorize(http.HandlerFunc(a.handlePDFPreview)))
	mux.Handle("/certificates/import", a.authorize(http.HandlerFunc(a.handleCertificatesImport)))
	mux.Handle("/certificates/validate", a.authorize(http.HandlerFunc(a.handleCertificateValidation)))
	mux.Handle("/certificates/online-check", a.authorize(http.HandlerFunc(a.handleCertificateOnlineCheck)))
	mux.Handle("/confianza/instalar", a.authorize(http.HandlerFunc(a.handleInstallPublicRoots)))
	mux.Handle("/trust/install-public-roots", a.authorize(http.HandlerFunc(a.handleInstallPublicRoots)))
	mux.Handle("/tls/trust-status", a.authorize(http.HandlerFunc(a.handleTLSTrustStatus)))
	mux.Handle("/diagnostics/report", a.authorize(http.HandlerFunc(a.handleDiagnosticsReport)))
	mux.Handle("/tls/clear-store", a.authorize(http.HandlerFunc(a.handleTLSClearStore)))
	mux.Handle("/sign", a.authorize(http.HandlerFunc(a.handleSign)))
	mux.Handle("/sign-batch", a.authorize(http.HandlerFunc(a.handleSignBatch)))
	mux.Handle("/hash", a.authorize(http.HandlerFunc(a.handleHashCreate)))
	mux.Handle("/hash/check", a.authorize(http.HandlerFunc(a.handleHashCheck)))
	mux.Handle("/protect", a.authorize(http.HandlerFunc(a.handleProtect)))
	mux.Handle("/protect-sign", a.authorize(http.HandlerFunc(a.handleProtectSign)))
	mux.Handle("/unprotect", a.authorize(http.HandlerFunc(a.handleUnprotect)))
	mux.Handle("/protection/recipients", a.authorize(http.HandlerFunc(a.handleProtectionRecipients)))
	mux.Handle("/protection/recipient/export", a.authorize(http.HandlerFunc(a.handleProtectionRecipientExport)))
	mux.Handle("/protection/recipient/import", a.authorize(http.HandlerFunc(a.handleProtectionRecipientImport)))
	mux.Handle("/verify", a.authorize(http.HandlerFunc(a.handleVerify)))
	mux.Handle("/select-certificate", a.authorize(http.HandlerFunc(a.handleSelectCertificate)))
	mux.HandleFunc("/signer", a.handleSigner)
	mux.HandleFunc("/firmador", a.handleSigner)
	mux.HandleFunc("/validator", a.handleValidator)
	mux.HandleFunc("/validador", a.handleValidator)
	mux.HandleFunc("/", a.handleIndex)
	limit := a.MaxBodyBytes
	if limit <= 0 {
		limit = 100 * 1024 * 1024
	}
	// H-07: el límite sigue siendo la capa exterior. La correlación se genera
	// después de acotar el body y antes de autorización y dispatch.
	return limitBodyMiddleware(restCorrelationMiddleware(mux), limit)
}

// limitBodyMiddleware limita el tamaño del body de toda petición entrante (CWE-770, H-07).
// Aplica http.MaxBytesReader antes de que cualquier handler lea r.Body.
func limitBodyMiddleware(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

type signRequest struct {
	RequestID     string            `json:"request_id"`
	Name          string            `json:"name"`
	ContentBase64 string            `json:"content_base64"`
	MIMEType      string            `json:"mime_type"`
	Format        string            `json:"format"`
	Action        string            `json:"action"`
	CertificateID string            `json:"certificate_id"`
	Options       map[string]string `json:"options"`
	InputPath     string            `json:"inputPath"`
	OutputPath    string            `json:"outputPath"`
	OriginalPath  string            `json:"originalPath"`
	Overwrite     string            `json:"overwrite"`
	CertificateIx *int              `json:"certificateIndex"`
	SaveToDisk    *bool             `json:"saveToDisk"`
	ReturnB64     *bool             `json:"returnSignatureB64"`
	StrictCompat  *bool             `json:"strictCompat"`
	AllowBadPDF   *bool             `json:"allowInvalidPDF"`

	requestIDPresent bool
}

type signResponse struct {
	OK                  bool   `json:"ok,omitempty"`
	RequestID           string `json:"request_id,omitempty"`
	Format              string `json:"format"`
	Algorithm           string `json:"algorithm"`
	SignedContentBase64 string `json:"signed_content_base64"`
	CertificateID       string `json:"certificate_id"`
	OutputPath          string `json:"outputPath,omitempty"`
}

type certificateOnlineCheckRequest struct {
	CertificateID string `json:"certificate_id"`
}

type certificateValidationRequest struct {
	CertificateID string `json:"certificate_id"`
}

type certificateValidationResponse struct {
	OK                    bool     `json:"ok"`
	Valid                 bool     `json:"valid"`
	CertificateID         string   `json:"certificate_id"`
	Subject               string   `json:"subject"`
	Issuer                string   `json:"issuer"`
	FingerprintSHA256     string   `json:"fingerprint_sha256"`
	NotBefore             string   `json:"not_before"`
	NotAfter              string   `json:"not_after"`
	TimeValid             bool     `json:"time_valid"`
	DigitalSignatureUsage bool     `json:"digital_signature_usage"`
	Trusted               bool     `json:"trusted"`
	ChainDepth            int      `json:"chain_depth"`
	PresentedChainDepth   int      `json:"presented_chain_depth"`
	Issues                []string `json:"issues,omitempty"`
}

type certificateOnlineCheckResponse struct {
	OK          bool   `json:"ok"`
	Status      string `json:"status"`
	UserMessage string `json:"userMessage,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Method      string `json:"method,omitempty"`
	CheckedAt   string `json:"checkedAt,omitempty"`
	RevokedAt   string `json:"revokedAt,omitempty"`
	OCSPURL     string `json:"ocspUrl,omitempty"`
	CRLURL      string `json:"crlUrl,omitempty"`
}

type signBatchRequest struct {
	RequestID     string                 `json:"request_id"`
	Items         []signBatchItemRequest `json:"items"`
	Format        string                 `json:"format"`
	Action        string                 `json:"action"`
	CertificateID string                 `json:"certificate_id"`
	Options       map[string]string      `json:"options"`
	Overwrite     string                 `json:"overwrite"`
	CertificateIx *int                   `json:"certificateIndex"`
	SaveToDisk    *bool                  `json:"saveToDisk"`
	ReturnB64     *bool                  `json:"returnSignatureB64"`
	StrictCompat  *bool                  `json:"strictCompat"`
	AllowBadPDF   *bool                  `json:"allowInvalidPDF"`

	requestIDPresent bool
}

type signBatchItemRequest struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
	MIMEType      string `json:"mime_type"`
	InputPath     string `json:"inputPath"`
	OutputPath    string `json:"outputPath"`
	// Options sobrescribe por clave la plantilla global Options del lote.
	Options map[string]string `json:"options,omitempty"`
}

type signBatchItemResponse struct {
	Index               int    `json:"index"`
	Name                string `json:"name"`
	OK                  bool   `json:"ok"`
	Format              string `json:"format,omitempty"`
	Algorithm           string `json:"algorithm,omitempty"`
	SignedContentBase64 string `json:"signed_content_base64,omitempty"`
	CertificateID       string `json:"certificate_id,omitempty"`
	OutputPath          string `json:"outputPath,omitempty"`
	Error               string `json:"error,omitempty"`
}

type signBatchResponse struct {
	OK           bool                    `json:"ok"`
	RequestID    string                  `json:"request_id,omitempty"`
	Total        int                     `json:"total"`
	SuccessCount int                     `json:"successCount"`
	FailureCount int                     `json:"failureCount"`
	Results      []signBatchItemResponse `json:"results"`
}

type protectRequest struct {
	Name          string            `json:"name"`
	ContentBase64 string            `json:"content_base64"`
	MIMEType      string            `json:"mime_type"`
	Profile       string            `json:"profile"`
	RecipientIDs  []string          `json:"recipient_ids"`
	RecipientID   string            `json:"recipient_id"`
	CertificateID string            `json:"certificate_id"`
	CertificateIx *int              `json:"certificateIndex"`
	SecretB64     string            `json:"secret_b64"`
	Options       map[string]string `json:"options"`
	InputPath     string            `json:"inputPath"`
	OutputPath    string            `json:"outputPath"`
	Overwrite     string            `json:"overwrite"`
	SaveToDisk    *bool             `json:"saveToDisk"`
	ReturnB64     *bool             `json:"returnProtectedB64"`
}

type protectResponse struct {
	OK                     bool   `json:"ok"`
	Profile                string `json:"profile"`
	RecipientCount         int    `json:"recipientCount"`
	CertificateID          string `json:"certificate_id,omitempty"`
	DocumentName           string `json:"documentName,omitempty"`
	MIMEType               string `json:"mime_type,omitempty"`
	ProtectedContentBase64 string `json:"protected_content_base64,omitempty"`
	OutputPath             string `json:"outputPath,omitempty"`
}

type unprotectRequest struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
	MIMEType      string `json:"mime_type"`
	SecretB64     string `json:"secret_b64"`
	InputPath     string `json:"inputPath"`
	OutputPath    string `json:"outputPath"`
	Overwrite     string `json:"overwrite"`
	SaveToDisk    *bool  `json:"saveToDisk"`
	ReturnB64     *bool  `json:"returnUnprotectedB64"`
}

type unprotectResponse struct {
	OK                       bool   `json:"ok"`
	Profile                  string `json:"profile"`
	RecipientID              string `json:"recipientId,omitempty"`
	DocumentName             string `json:"documentName,omitempty"`
	MIMEType                 string `json:"mime_type,omitempty"`
	UnprotectedContentBase64 string `json:"unprotected_content_base64,omitempty"`
	OutputPath               string `json:"outputPath,omitempty"`
}

type protectionRecipientEntryResponse struct {
	ID                          string `json:"id"`
	Label                       string `json:"label"`
	Profile                     string `json:"profile"`
	Algorithm                   string `json:"algorithm"`
	AuthEnvelopedDataCompatible bool   `json:"authEnvelopedDataCompatible"`
}

type protectionRecipientsResponse struct {
	OK         bool                               `json:"ok"`
	Recipients []protectionRecipientEntryResponse `json:"recipients"`
}

type protectionRecipientImportRequest struct {
	DataBase64 string `json:"data_base64"`
}

type protectionRecipientExchangeResponse struct {
	OK         bool                             `json:"ok"`
	Recipient  protectionRecipientEntryResponse `json:"recipient"`
	DataBase64 string                           `json:"data_base64,omitempty"`
	Filename   string                           `json:"filename,omitempty"`
}

type verifyRequest struct {
	Name           string `json:"name"`
	ContentBase64  string `json:"content_base64"`
	MIMEType       string `json:"mime_type"`
	OriginalBase64 string `json:"original_content_base64"`
	InputPath      string `json:"inputPath"`
	OriginalPath   string `json:"originalPath"`
}

type hashRequest struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
	InputPath     string `json:"inputPath"`
	Algorithm     string `json:"algorithm"`
	Format        string `json:"format"`
	OutputPath    string `json:"outputPath"`
	Overwrite     string `json:"overwrite"`
	SaveToDisk    *bool  `json:"saveToDisk"`
	Recursive     *bool  `json:"recursive"`
}

type hashResponse struct {
	OK         bool   `json:"ok"`
	Algorithm  string `json:"algorithm"`
	Format     string `json:"format"`
	Hash       string `json:"hash"`
	OutputPath string `json:"outputPath,omitempty"`
}

type hashCheckRequest struct {
	Name              string `json:"name"`
	ContentBase64     string `json:"content_base64"`
	InputPath         string `json:"inputPath"`
	HashContentBase64 string `json:"hash_content_base64"`
	HashPath          string `json:"hashPath"`
	Algorithm         string `json:"algorithm"`
	Recursive         *bool  `json:"recursive"`
	ReportOutputPath  string `json:"reportOutputPath"`
	Overwrite         string `json:"overwrite"`
	SaveReportToDisk  *bool  `json:"saveReportToDisk"`
}

type hashCheckResponse struct {
	OK           bool   `json:"ok"`
	Valid        bool   `json:"valid"`
	Algorithm    string `json:"algorithm"`
	Format       string `json:"format"`
	ExpectedHash string `json:"expectedHash"`
	ActualHash   string `json:"actualHash"`
}

type verifyResponse struct {
	OK      bool     `json:"ok,omitempty"`
	Valid   bool     `json:"valid"`
	Reason  string   `json:"reason"`
	Details []string `json:"details"`
	Signers []string `json:"signers"`
	Result  any      `json:"result,omitempty"`
}

type guidedDiagnosticResponse struct {
	Category               string `json:"category"`
	UserMessage            string `json:"userMessage"`
	ExpertMessage          string `json:"expertMessage"`
	LikelyOwner            string `json:"likelyOwner,omitempty"`
	ResponsibilityMessage  string `json:"responsibilityMessage,omitempty"`
	SuggestedAction        string `json:"suggestedAction,omitempty"`
	UserCanResolveDirectly bool   `json:"userCanResolveDirectly,omitempty"`
}

type errorResponse struct {
	Error      string                    `json:"error"`
	Diagnostic *guidedDiagnosticResponse `json:"diagnostic,omitempty"`
}

type verifyAspectResponse struct {
	Status  string   `json:"status"`
	Reason  string   `json:"reason,omitempty"`
	Details []string `json:"details,omitempty"`
}

type verifySignerSummaryResponse struct {
	ID          string `json:"id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type verifyEvidenceResponse struct {
	Type    string `json:"type"`
	Summary string `json:"summary,omitempty"`
}

type verifyRichResultResponse struct {
	Valid           bool                          `json:"valid"`
	Reason          string                        `json:"reason"`
	Details         []string                      `json:"details,omitempty"`
	Signers         []string                      `json:"signers,omitempty"`
	Format          string                        `json:"format,omitempty"`
	Coverage        string                        `json:"coverage,omitempty"`
	Integrity       verifyAspectResponse          `json:"integrity"`
	Certificate     verifyAspectResponse          `json:"certificate"`
	Trust           verifyAspectResponse          `json:"trust"`
	SignerSummaries []verifySignerSummaryResponse `json:"signerSummaries,omitempty"`
	Warnings        []string                      `json:"warnings,omitempty"`
	Errors          []string                      `json:"errors,omitempty"`
	Evidence        []verifyEvidenceResponse      `json:"evidence,omitempty"`
}

type selectCertificateRequest struct {
	SubjectFilter   string `json:"subject_filter"`
	IssuerFilter    string `json:"issuer_filter"`
	SoloNoCaducados bool   `json:"solo_no_caducados"`
}

type selectCertificateResponse struct {
	CertificateID string `json:"certificate_id"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	Confirmed     bool   `json:"confirmed"`
}

type restChallenge struct {
	Nonce      []byte
	ExpiresAt  time.Time
	Generation uint64
}

const maxPendingRESTChallenges = 128

type restSession struct {
	Token       string
	Subject     string
	Fingerprint string
	ExpiresAt   time.Time
	Generation  uint64
}

type authChallengeResponse struct {
	OK           bool   `json:"ok"`
	ChallengeID  string `json:"challengeId"`
	ChallengeB64 string `json:"challengeB64"`
	ExpiresAt    string `json:"expiresAt"`
}

type authVerifyRequest struct {
	ChallengeID      string `json:"challengeId"`
	ChallengeIDES    string `json:"idReto"`
	SignatureB64     string `json:"signatureB64"`
	SignatureB64ES   string `json:"firmaB64"`
	CertificatePEM   string `json:"certificatePEM"`
	CertificatePEMES string `json:"certificadoPEM"`
	CertificateB64   string `json:"certificateB64"`
	CertificateB64ES string `json:"certificadoB64"`
}

type authVerifyResponse struct {
	OK           bool   `json:"ok"`
	SessionToken string `json:"sessionToken"`
	ExpiresAt    string `json:"expiresAt"`
	Subject      string `json:"subject"`
	Fingerprint  string `json:"fingerprint"`
}

type healthResponse struct {
	OK      bool   `json:"ok"`
	Service string `json:"service"`
}

type certificateEntryResponse struct {
	ID           string `json:"id"`
	SubjectName  string `json:"subjectName"`
	IssuerName   string `json:"issuerName"`
	ValidTo      string `json:"validTo"`
	Fingerprint  string `json:"fingerprint"`
	SerialNumber string `json:"serialNumber"`
	Status       string `json:"status"`
	CanSign      bool   `json:"canSign"`
}

type certificatesResponse struct {
	OK           bool                       `json:"ok"`
	Certificates []certificateEntryResponse `json:"certificates"`
}

type settingsResponse struct {
	OK       bool           `json:"ok"`
	Settings map[string]any `json:"settings"`
}

type serviceStatusPayload struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Platform  string `json:"platform"`
	Method    string `json:"method"`
}

type serviceStatusResponse struct {
	OK     bool                 `json:"ok"`
	Status serviceStatusPayload `json:"status"`
}

type serviceActionResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type pdfPreviewRequest struct {
	Path string `json:"path"`
	Page int    `json:"page"`
}

type pdfPreviewResponse struct {
	OK           bool    `json:"ok"`
	ImageBase64  string  `json:"imageBase64,omitempty"`
	CurrentPage  int     `json:"currentPage,omitempty"`
	TotalPages   int     `json:"totalPages,omitempty"`
	WidthPoints  float64 `json:"widthPoints,omitempty"`
	HeightPoints float64 `json:"heightPoints,omitempty"`
	// Alias de compatibilidad con BackendBridge Qt anterior.
	Data   string  `json:"data,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	Error  string  `json:"error,omitempty"`
}

type importCertificateRequest struct {
	P12B64   string `json:"p12B64"`
	Password string `json:"password"`
}

type importCertificateResponse struct {
	OK          bool   `json:"ok"`
	Destination string `json:"destination,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Error       string `json:"error,omitempty"`
}

type publicRootsResponse struct {
	OK                    bool   `json:"ok"`
	LocalCertificateState string `json:"localCertificateState"`
	SystemTrustState      string `json:"systemTrustState"`
	Error                 string `json:"error,omitempty"`
}

type tlsTrustStatusResponse struct {
	OK       bool               `json:"ok"`
	TLSStore tlsStoreDiagnostic `json:"tlsStore"`
}

type tlsStoreDiagnostic struct {
	State                 string `json:"state"`
	ArtifactCount         int    `json:"artifactCount"`
	CertificateCount      int    `json:"certificateCount"`
	KeyCount              int    `json:"keyCount"`
	LocalCertificateState string `json:"localCertificateState"`
	SystemTrustState      string `json:"systemTrustState"`
}

type diagnosticsReportResponse struct {
	OK               bool               `json:"ok"`
	CertificateCount int                `json:"certificateCount"`
	CanSignCount     int                `json:"canSignCount"`
	TLSStore         tlsStoreDiagnostic `json:"tlsStore"`
}

type proxySecretStoreStatusResponse struct {
	OK          bool   `json:"ok"`
	Available   bool   `json:"available"`
	Platform    string `json:"platform,omitempty"`
	Backend     string `json:"backend,omitempty"`
	Reason      string `json:"reason,omitempty"`
	RuntimeMode string `json:"runtimeProxyMode,omitempty"`
	Error       string `json:"error,omitempty"`
}

type tlsClearStoreResponse struct {
	OK      bool   `json:"ok"`
	Removed int    `json:"removed"`
	Message string `json:"message,omitempty"`
}

const (
	maxVisibleSealImageBytes = 10 * 1024 * 1024
	maxPDFPreviewInputBytes  = 100 * 1024 * 1024
)

var errSignOptionPathDisabled = errors.New("las rutas de imagen de sello están deshabilitadas; use visibleSealImageBase64")

func (a *Adaptador) handleSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Firmar == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de firma no configurado")
		return
	}

	var req signRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de firma invalido")
		return
	}
	correlatedRequest, ok := withRESTRequestID(w, r, req.RequestID, req.requestIDPresent)
	if !ok {
		return
	}
	r = correlatedRequest
	if strings.TrimSpace(req.InputPath) != "" {
		if a.rejectPathOps(w) {
			return
		}
		a.handleSignByPath(w, r, req)
		return
	}

	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 no es valido")
		return
	}
	certificateID, err := a.resolveCertificateIDFromRequest(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	formato := inferirFormatoSolicitud(req.Format, req.Name)
	options, err := a.prepareSignOptionsWithDefaults(r.Context(), formato, req.Options)
	if err != nil {
		writeSignOptionsError(w, err)
		return
	}

	cmd, err := application.NewSignCommand(
		req.Name,
		content,
		req.MIMEType,
		formato,
		normalizarAccionFirmaTexto(req.Action),
		certificateID,
		options,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.acceptSignRequestID(w, req.RequestID) {
		return
	}
	result, err := a.Firmar.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, signResponse{
		OK:                  true,
		RequestID:           strings.TrimSpace(req.RequestID),
		Format:              string(result.Result.Format),
		Algorithm:           result.Result.Algorithm,
		SignedContentBase64: base64.StdEncoding.EncodeToString(result.Result.Data),
		CertificateID:       result.CertificateUsed.ID,
	})
}

func (a *Adaptador) handleSignByPath(w http.ResponseWriter, r *http.Request, req signRequest) {
	inputPath := strings.TrimSpace(req.InputPath)

	// H-01: Validar que la ruta no es un path traversal / acceso a sistema
	if errMsg := validatePathSecurity(inputPath); errMsg != "" {
		writeError(w, http.StatusBadRequest, "ruta no valida: "+errMsg)
		return
	}

	data, err := a.readPathFile(inputPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
		return
	}

	nombre := strings.TrimSpace(req.Name)
	if nombre == "" {
		nombre = filepath.Base(inputPath)
	}
	mimeType := strings.TrimSpace(req.MIMEType)
	if mimeType == "" {
		mimeType = inferirTipoMIMERuta(inputPath)
	}

	formato := inferirFormatoSolicitud(req.Format, inputPath)
	action := normalizarAccionFirmaTexto(req.Action)
	if strings.TrimSpace(action) == "" {
		action = "sign"
	}

	certificateID, err := a.resolveCertificateIDFromRequest(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	options, err := a.prepareSignOptionsWithDefaults(r.Context(), formato, req.Options)
	if err != nil {
		writeSignOptionsError(w, err)
		return
	}
	addBoolOption(options, "strictCompat", req.StrictCompat)
	addBoolOption(options, "allowInvalidPDF", req.AllowBadPDF)

	cmd, err := application.NewSignCommand(nombre, data, mimeType, formato, action, certificateID, options)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.acceptSignRequestID(w, req.RequestID) {
		return
	}
	result, err := a.Firmar.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saveToDisk := req.SaveToDisk == nil || *req.SaveToDisk
	outputPath := strings.TrimSpace(req.OutputPath)
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaRuta(inputPath, result.Result.Format)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err = writer.Escribir(outputPath, result.Result.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	resp := signResponse{
		OK:            true,
		RequestID:     strings.TrimSpace(req.RequestID),
		Format:        string(result.Result.Format),
		Algorithm:     result.Result.Algorithm,
		CertificateID: result.CertificateUsed.ID,
		OutputPath:    outputPath,
	}
	if req.ReturnB64 == nil || *req.ReturnB64 || !saveToDisk {
		resp.SignedContentBase64 = base64.StdEncoding.EncodeToString(result.Result.Data)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleSignBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.ProcesarLote == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de firma por lotes no configurado")
		return
	}

	var req signBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de firma múltiple invalido")
		return
	}
	correlatedRequest, ok := withRESTRequestID(w, r, req.RequestID, req.requestIDPresent)
	if !ok {
		return
	}
	r = correlatedRequest
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "debe indicar al menos un fichero")
		return
	}

	certificateID, err := a.resolveCertificateIDFromRequest(r.Context(), signRequest{
		CertificateID: req.CertificateID,
		CertificateIx: req.CertificateIx,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	baseOptions, err := a.prepareSignOptions(req.Options)
	if err != nil {
		writeSignOptionsError(w, err)
		return
	}
	addBoolOption(baseOptions, "strictCompat", req.StrictCompat)
	addBoolOption(baseOptions, "allowInvalidPDF", req.AllowBadPDF)
	var preferenciasFirma ports.DocumentoConfiguracionUsuario
	preferenciasCargadas := false

	type batchItemMeta struct {
		name       string
		inputPath  string
		outputPath string
		mimeType   string
	}

	inputs := make([]application.BatchItemInput, 0, len(req.Items))
	metas := make([]batchItemMeta, 0, len(req.Items))
	for _, itemReq := range req.Items {
		itemPath := strings.TrimSpace(itemReq.InputPath)
		if itemPath != "" || strings.TrimSpace(itemReq.OutputPath) != "" {
			if a.rejectPathOps(w) {
				return
			}
		}
		data, err := a.decodeBatchItemContent(itemReq)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(itemReq.Name)
		if name == "" && itemPath != "" {
			name = filepath.Base(itemPath)
		}
		if name == "" {
			writeError(w, http.StatusBadRequest, "cada fichero del lote debe tener nombre")
			return
		}
		mimeType := strings.TrimSpace(itemReq.MIMEType)
		if mimeType == "" {
			sourceName := itemPath
			if sourceName == "" {
				sourceName = name
			}
			mimeType = inferirTipoMIMERuta(sourceName)
		}
		sourceName := itemPath
		if sourceName == "" {
			sourceName = name
		}
		itemOptions, err := a.prepareSignOptions(itemReq.Options)
		if err != nil {
			writeSignOptionsError(w, err)
			return
		}
		formato := inferirFormatoSolicitud(req.Format, sourceName)
		explicitOptions := mergeSignOptions(baseOptions, itemOptions)
		if ports.NecesitaOpcionesFirmaPredeterminadas(formato, explicitOptions) && !preferenciasCargadas {
			// Las preferencias son un default de UX, no una dependencia de
			// disponibilidad: si están dañadas, el motor conserva ETSI seguro.
			preferenciasFirma, _ = a.loadSigningPreferences(r.Context())
			preferenciasCargadas = true
		}
		itemOptions = ports.AplicarOpcionesFirmaPredeterminadas(preferenciasFirma, formato, explicitOptions)
		inputs = append(inputs, application.BatchItemInput{
			Nombre:    name,
			Contenido: data,
			TipoMIME:  mimeType,
			Formato:   formato,
			Accion:    normalizarAccionFirmaTexto(req.Action),
			Opciones:  itemOptions,
		})
		metas = append(metas, batchItemMeta{
			name:       name,
			inputPath:  itemPath,
			outputPath: strings.TrimSpace(itemReq.OutputPath),
			mimeType:   mimeType,
		})
	}

	cmd, err := application.NewProcessBatchCommand(inputs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cmd.CertificateID = certificateID

	if !a.acceptSignRequestID(w, req.RequestID) {
		return
	}
	result, err := a.ProcesarLote.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saveToDisk := req.SaveToDisk != nil && *req.SaveToDisk
	results := make([]signBatchItemResponse, 0, len(metas))
	successIndex := 0
	successCount := 0
	failureCount := 0
	for i, meta := range metas {
		itemResp := signBatchItemResponse{
			Index: i,
			Name:  meta.name,
		}
		if itemErr, failed := result.Errores[i]; failed {
			itemResp.Error = itemErr.Error()
			failureCount++
			results = append(results, itemResp)
			continue
		}
		if successIndex >= len(result.Results) {
			itemResp.Error = "resultado de lote inconsistente"
			failureCount++
			results = append(results, itemResp)
			continue
		}

		signed := result.Results[successIndex]
		successIndex++
		itemResp.OK = true
		itemResp.Format = string(signed.Result.Format)
		itemResp.Algorithm = signed.Result.Algorithm
		itemResp.CertificateID = signed.CertificateUsed.ID

		wantsDisk := saveToDisk && (meta.inputPath != "" || meta.outputPath != "")
		outputPath := meta.outputPath
		if wantsDisk && outputPath == "" && meta.inputPath != "" {
			outputPath = construirSalidaRuta(meta.inputPath, signed.Result.Format)
		}
		if wantsDisk && outputPath != "" {
			politica := filesystem.PoliticaRenombrar
			if valorBoolTexto(req.Overwrite) {
				politica = filesystem.PoliticaForzar
			}
			writer := filesystem.NuevoEscritorResultado(politica)
			realPath, writeErr := writer.Escribir(outputPath, signed.Result.Data)
			if writeErr != nil {
				itemResp.OK = false
				itemResp.Error = writeErr.Error()
				failureCount++
				if req.ReturnB64 == nil || *req.ReturnB64 {
					itemResp.SignedContentBase64 = base64.StdEncoding.EncodeToString(signed.Result.Data)
				}
				results = append(results, itemResp)
				continue
			}
			itemResp.OutputPath = realPath
		}

		if req.ReturnB64 == nil || *req.ReturnB64 || !wantsDisk || itemResp.OutputPath == "" {
			itemResp.SignedContentBase64 = base64.StdEncoding.EncodeToString(signed.Result.Data)
		}
		successCount++
		results = append(results, itemResp)
	}

	writeJSON(w, http.StatusOK, signBatchResponse{
		OK:           failureCount == 0,
		RequestID:    strings.TrimSpace(req.RequestID),
		Total:        len(results),
		SuccessCount: successCount,
		FailureCount: failureCount,
		Results:      results,
	})
}

func (a *Adaptador) handleProtect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Proteger == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de proteccion no configurado")
		return
	}

	var req protectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de proteccion invalido")
		return
	}
	if strings.TrimSpace(req.InputPath) != "" {
		if a.rejectPathOps(w) {
			return
		}
		a.handleProtectByPath(w, r, req)
		return
	}

	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 no es valido")
		return
	}
	cmd, err := application.NewProtectCommand(
		req.Name,
		content,
		req.MIMEType,
		normalizarPerfilProteccion(req.Profile),
		normalizarDestinatariosProteccion(req.RecipientID, req.RecipientIDs),
		normalizarOpcionesProteccion(req.Options, req.SecretB64),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.Proteger.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, protectResponse{
		OK:                     true,
		Profile:                string(result.Protected.Profile),
		RecipientCount:         result.Protected.RecipientCount,
		DocumentName:           result.Protected.Document.Name,
		MIMEType:               result.Protected.Document.MIMEType,
		ProtectedContentBase64: base64.StdEncoding.EncodeToString(result.Protected.Document.Content),
	})
}

func (a *Adaptador) handleProtectSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.ProtegerFirmando == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de proteccion firmada no configurado")
		return
	}

	var req protectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de proteccion firmada invalido")
		return
	}
	if strings.TrimSpace(req.InputPath) != "" {
		if a.rejectPathOps(w) {
			return
		}
		a.handleProtectSignByPath(w, r, req)
		return
	}

	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 no es valido")
		return
	}
	certificateID, err := a.resolveProtectionCertificateID(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	options := normalizarOpcionesProteccion(req.Options, req.SecretB64)
	if err := normalizarContenedorProteccionFirmada(options); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cmd, err := application.NewProtectAndSignCommand(
		req.Name,
		content,
		req.MIMEType,
		normalizarPerfilProteccion(req.Profile),
		normalizarDestinatariosProteccion(req.RecipientID, req.RecipientIDs),
		certificateID,
		options,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.ProtegerFirmando.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, protectResponse{
		OK:                     true,
		Profile:                string(result.Protected.Profile),
		RecipientCount:         result.Protected.RecipientCount,
		CertificateID:          result.CertificateUsed.ID,
		DocumentName:           result.Protected.Document.Name,
		MIMEType:               result.Protected.Document.MIMEType,
		ProtectedContentBase64: base64.StdEncoding.EncodeToString(result.Protected.Document.Content),
	})
}

func (a *Adaptador) handleProtectSignByPath(w http.ResponseWriter, r *http.Request, req protectRequest) {
	inputPath := strings.TrimSpace(req.InputPath)

	// H-01: Validar que la ruta no es un path traversal / acceso a sistema
	if errMsg := validatePathSecurity(inputPath); errMsg != "" {
		writeError(w, http.StatusBadRequest, "ruta no valida: "+errMsg)
		return
	}

	data, err := a.readPathFile(inputPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(inputPath)
	}
	mimeType := strings.TrimSpace(req.MIMEType)
	if mimeType == "" {
		mimeType = inferirTipoMIMERuta(inputPath)
	}
	certificateID, err := a.resolveProtectionCertificateID(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	options := normalizarOpcionesProteccion(req.Options, req.SecretB64)
	if err := normalizarContenedorProteccionFirmada(options); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cmd, err := application.NewProtectAndSignCommand(
		name,
		data,
		mimeType,
		normalizarPerfilProteccion(req.Profile),
		normalizarDestinatariosProteccion(req.RecipientID, req.RecipientIDs),
		certificateID,
		options,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.ProtegerFirmando.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saveToDisk := req.SaveToDisk == nil || *req.SaveToDisk
	outputPath := strings.TrimSpace(req.OutputPath)
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaProtegidaRuta(inputPath, result.Protected.Document.Name)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err = writer.Escribir(outputPath, result.Protected.Document.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	resp := protectResponse{
		OK:             true,
		Profile:        string(result.Protected.Profile),
		RecipientCount: result.Protected.RecipientCount,
		CertificateID:  result.CertificateUsed.ID,
		DocumentName:   result.Protected.Document.Name,
		MIMEType:       result.Protected.Document.MIMEType,
		OutputPath:     outputPath,
	}
	if req.ReturnB64 == nil || *req.ReturnB64 || !saveToDisk {
		resp.ProtectedContentBase64 = base64.StdEncoding.EncodeToString(result.Protected.Document.Content)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleProtectByPath(w http.ResponseWriter, r *http.Request, req protectRequest) {
	inputPath := strings.TrimSpace(req.InputPath)

	// H-01: Validar que la ruta no es un path traversal / acceso a sistema
	if errMsg := validatePathSecurity(inputPath); errMsg != "" {
		writeError(w, http.StatusBadRequest, "ruta no valida: "+errMsg)
		return
	}

	data, err := a.readPathFile(inputPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(inputPath)
	}
	mimeType := strings.TrimSpace(req.MIMEType)
	if mimeType == "" {
		mimeType = inferirTipoMIMERuta(inputPath)
	}
	cmd, err := application.NewProtectCommand(
		name,
		data,
		mimeType,
		normalizarPerfilProteccion(req.Profile),
		normalizarDestinatariosProteccion(req.RecipientID, req.RecipientIDs),
		normalizarOpcionesProteccion(req.Options, req.SecretB64),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.Proteger.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saveToDisk := req.SaveToDisk == nil || *req.SaveToDisk
	outputPath := strings.TrimSpace(req.OutputPath)
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaProtegidaRuta(inputPath, result.Protected.Document.Name)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err = writer.Escribir(outputPath, result.Protected.Document.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	resp := protectResponse{
		OK:             true,
		Profile:        string(result.Protected.Profile),
		RecipientCount: result.Protected.RecipientCount,
		DocumentName:   result.Protected.Document.Name,
		MIMEType:       result.Protected.Document.MIMEType,
		OutputPath:     outputPath,
	}
	if req.ReturnB64 == nil || *req.ReturnB64 || !saveToDisk {
		resp.ProtectedContentBase64 = base64.StdEncoding.EncodeToString(result.Protected.Document.Content)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleUnprotect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Desproteger == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de desproteccion no configurado")
		return
	}

	var req unprotectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de desproteccion invalido")
		return
	}
	if strings.TrimSpace(req.InputPath) != "" {
		if a.rejectPathOps(w) {
			return
		}
		a.handleUnprotectByPath(w, r, req)
		return
	}

	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 no es valido")
		return
	}
	doc, err := domain.NewDocument(req.Name, content, normalizarMIMEProtegido(req.Name, req.MIMEType))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.Desproteger.Execute(r.Context(), application.UnprotectCommand{
		ProtectedDocument: doc,
		Options:           normalizarOpcionesProteccion(nil, req.SecretB64),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, unprotectResponse{
		OK:                       true,
		Profile:                  string(result.Unprotected.Profile),
		RecipientID:              result.Unprotected.RecipientID,
		DocumentName:             result.Unprotected.Document.Name,
		MIMEType:                 result.Unprotected.Document.MIMEType,
		UnprotectedContentBase64: base64.StdEncoding.EncodeToString(result.Unprotected.Document.Content),
	})
}

func (a *Adaptador) handleUnprotectByPath(w http.ResponseWriter, r *http.Request, req unprotectRequest) {
	inputPath := strings.TrimSpace(req.InputPath)

	// H-01: Validar que la ruta no es un path traversal / acceso a sistema
	if errMsg := validatePathSecurity(inputPath); errMsg != "" {
		writeError(w, http.StatusBadRequest, "ruta no valida: "+errMsg)
		return
	}

	data, err := a.readPathFile(inputPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
		return
	}
	doc, err := domain.NewDocument(filepath.Base(inputPath), data, normalizarMIMEProtegido(filepath.Base(inputPath), req.MIMEType))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.Desproteger.Execute(r.Context(), application.UnprotectCommand{
		ProtectedDocument: doc,
		Options:           normalizarOpcionesProteccion(nil, req.SecretB64),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saveToDisk := req.SaveToDisk == nil || *req.SaveToDisk
	outputPath := strings.TrimSpace(req.OutputPath)
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaDesprotegidaRuta(inputPath, result.Unprotected.Document.Name)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err = writer.Escribir(outputPath, result.Unprotected.Document.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	resp := unprotectResponse{
		OK:           true,
		Profile:      string(result.Unprotected.Profile),
		RecipientID:  result.Unprotected.RecipientID,
		DocumentName: result.Unprotected.Document.Name,
		MIMEType:     result.Unprotected.Document.MIMEType,
		OutputPath:   outputPath,
	}
	if req.ReturnB64 == nil || *req.ReturnB64 || !saveToDisk {
		resp.UnprotectedContentBase64 = base64.StdEncoding.EncodeToString(result.Unprotected.Document.Content)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleProtectionRecipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Destinatarios == nil {
		writeError(w, http.StatusNotImplemented, "catalogo de destinatarios de proteccion no configurado")
		return
	}
	recipients, err := a.Destinatarios.List(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := make([]protectionRecipientEntryResponse, 0, len(recipients))
	for _, recipient := range recipients {
		out = append(out, makeProtectionRecipientEntryResponse(recipient))
	}
	writeJSON(w, http.StatusOK, protectionRecipientsResponse{
		OK:         true,
		Recipients: out,
	})
}

func (a *Adaptador) handleProtectionRecipientExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.ExportarProteccion == nil {
		writeError(w, http.StatusNotImplemented, "intercambio de destinatarios de proteccion no configurado")
		return
	}
	recipientID := strings.TrimSpace(r.URL.Query().Get("id"))
	if recipientID == "" {
		writeError(w, http.StatusBadRequest, "id es obligatorio")
		return
	}
	result, err := a.ExportarProteccion.Execute(r.Context(), application.ExportProtectionRecipientCommand{
		RecipientID: recipientID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, protectionRecipientExchangeResponse{
		OK:         true,
		Recipient:  makeProtectionRecipientEntryResponse(result.Recipient),
		DataBase64: base64.StdEncoding.EncodeToString(result.Data),
		Filename:   result.Recipient.ID + ".afpr.json",
	})
}

func (a *Adaptador) handleProtectionRecipientImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.ImportarProteccion == nil {
		writeError(w, http.StatusNotImplemented, "intercambio de destinatarios de proteccion no configurado")
		return
	}
	var req protectionRecipientImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json invalido")
		return
	}
	dataB64 := strings.TrimSpace(req.DataBase64)
	if dataB64 == "" {
		writeError(w, http.StatusBadRequest, "data_base64 es obligatorio")
		return
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "data_base64 invalido")
		return
	}
	result, err := a.ImportarProteccion.Execute(r.Context(), application.ImportProtectionRecipientCommand{
		Data: data,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, protectionRecipientExchangeResponse{
		OK:        true,
		Recipient: makeProtectionRecipientEntryResponse(result.Recipient),
	})
}

func (a *Adaptador) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Verificar == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de verificacion no configurado")
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de verificacion invalido")
		return
	}
	if strings.TrimSpace(req.InputPath) != "" {
		if a.rejectPathOps(w) {
			return
		}
		a.handleVerifyByPath(w, r, req)
		return
	}
	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 no es valido")
		return
	}
	doc, err := domain.NewDocument(req.Name, content, req.MIMEType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cmd := application.VerifyCommand{SignedDocument: doc}
	if strings.TrimSpace(req.OriginalBase64) != "" {
		originalContent, err := base64.StdEncoding.DecodeString(req.OriginalBase64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "original_content_base64 no es valido")
			return
		}
		originalDoc, err := domain.NewDocument(req.Name, originalContent, inferOriginalMIMEType(req.MIMEType))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cmd.OriginalDocument = &originalDoc
	}
	result, err := a.Verificar.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := buildVerifyResponse(result)
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleHashCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.CrearHash == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de huellas no configurado")
		return
	}

	var req hashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de huella invalido")
		return
	}
	name := strings.TrimSpace(req.Name)
	data := []byte(nil)
	switch {
	case strings.TrimSpace(req.InputPath) != "":
		if a.rejectPathOps(w) {
			return
		}
		path := strings.TrimSpace(req.InputPath)
		if errMsg := validatePathSecurity(path); errMsg != "" {
			writeError(w, http.StatusBadRequest, "inputPath no valida: "+errMsg)
			return
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			a.handleDirectoryHashCreate(w, r, req, path)
			return
		}
		content, err := a.readPathFile(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
			return
		}
		if name == "" {
			name = filepath.Base(path)
		}
		data = content
	default:
		content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "content_base64 no es valido")
			return
		}
		data = content
	}
	format, err := application.ParseHashOutputFormat(req.Format)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.CrearHash.Execute(r.Context(), application.CreateHashCommand{
		Data:      data,
		Algorithm: req.Algorithm,
		Format:    format,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := hashResponse{
		OK:        true,
		Algorithm: result.Algorithm,
		Format:    string(result.Format),
		Hash:      result.Encoded,
	}
	saveToDisk := req.SaveToDisk == nil || *req.SaveToDisk
	if saveToDisk && strings.TrimSpace(req.OutputPath) != "" {
		if a.rejectPathOps(w) {
			return
		}
		outputPath := strings.TrimSpace(req.OutputPath)
		if errMsg := validatePathSecurity(outputPath); errMsg != "" {
			writeError(w, http.StatusBadRequest, "outputPath no valida: "+errMsg)
			return
		}
		payload := []byte(result.Encoded)
		if result.Format == application.HashFormatBinary {
			payload = append([]byte(nil), result.Digest...)
		}
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err := writer.Escribir(outputPath, payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp.OutputPath = outputPath
	} else if saveToDisk && strings.TrimSpace(req.InputPath) != "" {
		outputPath := filepath.Join(filepath.Dir(strings.TrimSpace(req.InputPath)), filepath.Base(defaultHashFilename(name, result.Format)))
		if errMsg := validatePathSecurity(outputPath); errMsg != "" {
			writeError(w, http.StatusBadRequest, "outputPath no valida: "+errMsg)
			return
		}
		payload := []byte(result.Encoded)
		if result.Format == application.HashFormatBinary {
			payload = append([]byte(nil), result.Digest...)
		}
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err := writer.Escribir(outputPath, payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp.OutputPath = outputPath
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleHashCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}

	var req hashCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de comprobacion invalido")
		return
	}

	var data []byte
	switch {
	case strings.TrimSpace(req.InputPath) != "":
		if a.rejectPathOps(w) {
			return
		}
		inputPath := strings.TrimSpace(req.InputPath)
		if errMsg := validatePathSecurity(inputPath); errMsg != "" {
			writeError(w, http.StatusBadRequest, "inputPath no valida: "+errMsg)
			return
		}
		if info, err := os.Stat(inputPath); err == nil && info.IsDir() {
			a.handleDirectoryHashCheck(w, r, req, inputPath)
			return
		}
		content, err := a.readPathFile(inputPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
			return
		}
		data = content
	default:
		content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "content_base64 no es valido")
			return
		}
		data = content
	}
	if a.ComprobarHash == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de comprobacion de huellas no configurado")
		return
	}

	var hashRaw []byte
	hashHint := strings.TrimSpace(req.HashPath)
	switch {
	case hashHint != "":
		if a.rejectPathOps(w) {
			return
		}
		if errMsg := validatePathSecurity(hashHint); errMsg != "" {
			writeError(w, http.StatusBadRequest, "hashPath no valida: "+errMsg)
			return
		}
		content, err := a.readPathFile(hashHint)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no se pudo leer hashPath")
			return
		}
		hashRaw = content
	default:
		content, err := base64.StdEncoding.DecodeString(req.HashContentBase64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "hash_content_base64 no es valido")
			return
		}
		hashRaw = content
	}

	expectedDigest, inferredAlg, inferredFormat, err := application.ParseStoredHash(hashRaw, hashHint)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	algorithm := inferredAlg
	if trimmed := strings.TrimSpace(req.Algorithm); trimmed != "" {
		algorithm = trimmed
	}
	result, err := a.ComprobarHash.Execute(r.Context(), application.CheckHashCommand{
		Data:         data,
		ExpectedHash: expectedDigest,
		Algorithm:    algorithm,
		Format:       inferredFormat,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hashCheckResponse{
		OK:           true,
		Valid:        result.Valid,
		Algorithm:    result.Algorithm,
		Format:       string(result.Format),
		ExpectedHash: result.ExpectedEncoded,
		ActualHash:   result.ActualEncoded,
	})
}

func (a *Adaptador) handleDirectoryHashCreate(w http.ResponseWriter, r *http.Request, req hashRequest, inputPath string) {
	if a.CrearHashDir == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de hash de directorio no configurado")
		return
	}
	format, err := parseDirectoryHashRequestFormat(req.Format)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.CrearHashDir.Execute(r.Context(), application.CreateDirectoryHashManifestCommand{
		RootPath:  inputPath,
		Algorithm: req.Algorithm,
		Format:    format,
		Recursive: req.Recursive != nil && *req.Recursive,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := map[string]any{
		"ok":              true,
		"algorithm":       result.Algorithm,
		"format":          string(result.Format),
		"entries":         len(result.Manifest.Entries),
		"recursive":       result.Manifest.Recursive,
		"manifest_base64": base64.StdEncoding.EncodeToString(result.Data),
	}
	saveToDisk := req.SaveToDisk == nil || *req.SaveToDisk
	if saveToDisk {
		outputPath := strings.TrimSpace(req.OutputPath)
		if outputPath == "" {
			outputPath = filepath.Join(filepath.Dir(inputPath), filepath.Base(defaultDirectoryHashFilename(filepath.Base(inputPath), result.Format)))
		}
		if errMsg := validatePathSecurity(outputPath); errMsg != "" {
			writeError(w, http.StatusBadRequest, "outputPath no valida: "+errMsg)
			return
		}
		politica := filesystem.PoliticaRenombrar
		if valorBoolTexto(req.Overwrite) {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		outputPath, err = writer.Escribir(outputPath, result.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp["outputPath"] = outputPath
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleDirectoryHashCheck(w http.ResponseWriter, r *http.Request, req hashCheckRequest, inputPath string) {
	if a.ComprobarHashDir == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de comprobacion de hash de directorio no configurado")
		return
	}
	var hashRaw []byte
	hashHint := strings.TrimSpace(req.HashPath)
	switch {
	case hashHint != "":
		if errMsg := validatePathSecurity(hashHint); errMsg != "" {
			writeError(w, http.StatusBadRequest, "hashPath no valida: "+errMsg)
			return
		}
		content, err := a.readPathFile(hashHint)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no se pudo leer hashPath")
			return
		}
		hashRaw = content
	default:
		content, err := base64.StdEncoding.DecodeString(req.HashContentBase64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "hash_content_base64 no es valido")
			return
		}
		hashRaw = content
	}
	result, err := a.ComprobarHashDir.Execute(r.Context(), application.CheckDirectoryHashManifestCommand{
		RootPath:     inputPath,
		ManifestData: hashRaw,
		ManifestHint: hashHint,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := map[string]any{
		"ok":                true,
		"valid":             result.Valid,
		"algorithm":         result.Report.Algorithm,
		"recursive":         result.Report.Recursive,
		"matching_hash":     result.Report.MatchingHash,
		"not_matching_hash": result.Report.NotMatchingHash,
		"hash_without_file": result.Report.HashWithoutFile,
		"file_without_hash": result.Report.FileWithoutHash,
	}
	if a.InformeHashDir != nil {
		reportData, err := a.InformeHashDir.EncodeReport(r.Context(), result.Report)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "no se pudo generar el informe de comprobacion")
			return
		}
		resp["report_base64"] = base64.StdEncoding.EncodeToString(reportData)
		saveToDisk := req.SaveReportToDisk != nil && *req.SaveReportToDisk
		if saveToDisk {
			outputPath := strings.TrimSpace(req.ReportOutputPath)
			if outputPath == "" {
				outputPath = filepath.Join(filepath.Dir(inputPath), filepath.Base(defaultDirectoryHashReportFilename(filepath.Base(inputPath))))
			}
			if errMsg := validatePathSecurity(outputPath); errMsg != "" {
				writeError(w, http.StatusBadRequest, "reportOutputPath no valida: "+errMsg)
				return
			}
			politica := filesystem.PoliticaRenombrar
			if valorBoolTexto(req.Overwrite) {
				politica = filesystem.PoliticaForzar
			}
			writer := filesystem.NuevoEscritorResultado(politica)
			outputPath, err = writer.Escribir(outputPath, reportData)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			resp["report_output_path"] = outputPath
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adaptador) handleVerifyByPath(w http.ResponseWriter, r *http.Request, req verifyRequest) {
	inputPath := strings.TrimSpace(req.InputPath)

	// H-01: Validar que la ruta no es un path traversal / acceso a sistema
	if errMsg := validatePathSecurity(inputPath); errMsg != "" {
		writeError(w, http.StatusBadRequest, "ruta no valida: "+errMsg)
		return
	}

	signedData, err := a.readPathFile(inputPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no se pudo leer inputPath")
		return
	}
	signedDoc, err := domain.NewDocument(filepath.Base(inputPath), signedData, inferirTipoMIMERuta(inputPath))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cmd := application.VerifyCommand{SignedDocument: signedDoc}
	if originalPath := strings.TrimSpace(req.OriginalPath); originalPath != "" {
		// H-01: Validar también originalPath
		if errMsg := validatePathSecurity(originalPath); errMsg != "" {
			writeError(w, http.StatusBadRequest, "originalPath no valida: "+errMsg)
			return
		}
		originalData, err := a.readPathFile(originalPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no se pudo leer originalPath")
			return
		}
		originalDoc, err := domain.NewDocument(filepath.Base(originalPath), originalData, inferirTipoMIMERuta(originalPath))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cmd.OriginalDocument = &originalDoc
	}

	result, err := a.Verificar.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := buildVerifyResponse(result)
	writeJSON(w, http.StatusOK, resp)
}

func buildVerifyResponse(result application.VerifyResult) verifyResponse {
	signers := make([]string, 0, len(result.Firmantes))
	for _, signer := range result.Firmantes {
		signers = append(signers, signer.ID)
	}
	return verifyResponse{
		OK:      true,
		Valid:   result.Verification.Valid,
		Reason:  result.Verification.Reason,
		Details: append([]string(nil), result.Verification.Details...),
		Signers: signers,
		Result:  buildVerifyRichResult(result.Verification, result.Firmantes, signers),
	}
}

func buildVerifyRichResult(v domain.VerificationResult, signerRefs []domain.CertificateRef, signers []string) verifyRichResultResponse {
	out := verifyRichResultResponse{
		Valid:    v.Valid,
		Reason:   v.Reason,
		Details:  append([]string(nil), v.Details...),
		Signers:  append([]string(nil), signers...),
		Format:   strings.TrimSpace(v.Format),
		Coverage: strings.TrimSpace(v.Coverage),
		Integrity: verifyAspectResponse{
			Status:  string(v.Integrity.Status),
			Reason:  v.Integrity.Reason,
			Details: append([]string(nil), v.Integrity.Details...),
		},
		Certificate: verifyAspectResponse{
			Status:  string(v.Certificate.Status),
			Reason:  v.Certificate.Reason,
			Details: append([]string(nil), v.Certificate.Details...),
		},
		Trust: verifyAspectResponse{
			Status:  string(v.Trust.Status),
			Reason:  v.Trust.Reason,
			Details: append([]string(nil), v.Trust.Details...),
		},
		Warnings: append([]string(nil), v.Warnings...),
		Errors:   append([]string(nil), v.Errors...),
	}
	if len(v.SignerSummaries) > 0 {
		out.SignerSummaries = make([]verifySignerSummaryResponse, 0, len(v.SignerSummaries))
		for _, signer := range v.SignerSummaries {
			out.SignerSummaries = append(out.SignerSummaries, verifySignerSummaryResponse{
				ID:          signer.ID,
				Subject:     signer.Subject,
				Issuer:      signer.Issuer,
				Fingerprint: signer.Fingerprint,
			})
		}
	} else if len(signerRefs) > 0 {
		out.SignerSummaries = make([]verifySignerSummaryResponse, 0, len(signerRefs))
		for _, signer := range signerRefs {
			out.SignerSummaries = append(out.SignerSummaries, verifySignerSummaryResponse{
				ID:          signer.ID,
				Subject:     signer.Subject,
				Issuer:      signer.Issuer,
				Fingerprint: signer.Fingerprint,
			})
		}
	}
	if len(v.Evidence) > 0 {
		out.Evidence = make([]verifyEvidenceResponse, 0, len(v.Evidence))
		for _, evidence := range v.Evidence {
			out.Evidence = append(out.Evidence, verifyEvidenceResponse{
				Type:    evidence.Type,
				Summary: evidence.Summary,
			})
		}
	}
	return out
}

func defaultHashFilename(name string, format application.HashOutputFormat) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "documento"
	}
	switch format {
	case application.HashFormatBinary:
		return base + ".hash"
	case application.HashFormatBase64:
		return base + ".hashb64"
	default:
		return base + ".hexhash"
	}
}

func defaultDirectoryHashFilename(name string, format domain.DirectoryHashManifestFormat) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "directorio"
	}
	switch format {
	case domain.DirectoryHashFormatTXT:
		return base + ".txthashfiles"
	case domain.DirectoryHashFormatCSV:
		return base + ".csv"
	default:
		return base + ".hashfiles"
	}
}

func defaultDirectoryHashReportFilename(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "directorio"
	}
	return base + ".hashreport"
}

func parseDirectoryHashRequestFormat(raw string) (domain.DirectoryHashManifestFormat, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "xml", "hashfiles":
		return domain.DirectoryHashFormatXML, nil
	case "txt", "txthashfiles":
		return domain.DirectoryHashFormatTXT, nil
	case "csv":
		return domain.DirectoryHashFormatCSV, nil
	default:
		return "", errors.New("formato de manifiesto no soportado")
	}
}

func (a *Adaptador) handleSelectCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.SeleccionarCert == nil {
		writeError(w, http.StatusNotImplemented, "caso de uso de seleccion no configurado")
		return
	}

	var req selectCertificateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, context.Canceled) {
		writeError(w, http.StatusBadRequest, "json de seleccion invalido")
		return
	}

	result, err := a.SeleccionarCert.Execute(r.Context(), application.SelectCertificateCommand{
		SubjectFilter:   req.SubjectFilter,
		IssuerFilter:    req.IssuerFilter,
		SoloNoCaducados: req.SoloNoCaducados,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, selectCertificateResponse{
		CertificateID: result.Selection.Certificate.ID,
		Subject:       result.Selection.Certificate.Subject,
		Issuer:        result.Selection.Certificate.Issuer,
		Confirmed:     result.Selection.Confirmed,
	})
}

func (a *Adaptador) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	enabled, generation := a.certificateAuthSnapshot()
	if !enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	a.cleanupExpiredAuthState()
	challengeID, challengeRaw, err := newRandomTokenPair()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo generar reto")
		return
	}
	exp := time.Now().Add(2 * time.Minute)
	a.mu.Lock()
	if !a.CertificateAuthEnabled || a.authGeneration != generation {
		a.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if len(a.challenges) >= maxPendingRESTChallenges {
		a.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "demasiados retos de autenticacion pendientes")
		return
	}
	a.challenges[challengeID] = restChallenge{
		Nonce:      challengeRaw,
		ExpiresAt:  exp,
		Generation: generation,
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, authChallengeResponse{
		OK:           true,
		ChallengeID:  challengeID,
		ChallengeB64: base64.StdEncoding.EncodeToString(challengeRaw),
		ExpiresAt:    exp.Format(time.RFC3339),
	})
}

func (a *Adaptador) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	enabled, generation := a.certificateAuthSnapshot()
	if !enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	var req authVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json invalido")
		return
	}
	normalizeAuthVerifyRequestAliases(&req)
	challengeID := strings.TrimSpace(req.ChallengeID)
	signatureB64 := strings.TrimSpace(req.SignatureB64)
	if challengeID == "" || signatureB64 == "" {
		writeError(w, http.StatusBadRequest, "challengeId y signatureB64 son obligatorios")
		return
	}

	a.cleanupExpiredAuthState()
	a.mu.Lock()
	if !a.CertificateAuthEnabled || a.authGeneration != generation {
		a.mu.Unlock()
		writeError(w, http.StatusForbidden, "politica de autenticacion modificada")
		return
	}
	ch, ok := a.challenges[challengeID]
	if ok {
		delete(a.challenges, challengeID)
	}
	a.mu.Unlock()
	if !ok || ch.Generation != generation {
		writeError(w, http.StatusBadRequest, "challenge invalido o expirado")
		return
	}

	cert, fp, err := parseAuthCertificate(strings.TrimSpace(req.CertificatePEM), strings.TrimSpace(req.CertificateB64))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	normalizedFingerprint := strings.ToLower(fp)
	a.mu.Lock()
	_, allowed := a.AllowedCerts[normalizedFingerprint]
	configUnchanged := a.CertificateAuthEnabled && a.authGeneration == generation
	a.mu.Unlock()
	if !configUnchanged || !allowed {
		writeError(w, http.StatusForbidden, "certificado no autorizado")
		return
	}
	if now := time.Now(); now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		writeError(w, http.StatusForbidden, "certificado fuera de vigencia")
		return
	}
	sigRaw, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signatureB64 invalida")
		return
	}
	if err := verifyChallengeSignature(cert, ch.Nonce, sigRaw); err != nil {
		writeError(w, http.StatusForbidden, "firma de reto invalida")
		return
	}

	sessionToken, _, err := newRandomTokenPair()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo crear sesión")
		return
	}
	subj := strings.TrimSpace(cert.Subject.String())
	a.mu.Lock()
	if !a.CertificateAuthEnabled || a.authGeneration != generation {
		a.mu.Unlock()
		writeError(w, http.StatusForbidden, "politica de autenticacion modificada")
		return
	}
	if _, allowed := a.AllowedCerts[normalizedFingerprint]; !allowed {
		a.mu.Unlock()
		writeError(w, http.StatusForbidden, "certificado no autorizado")
		return
	}
	ttl := a.SessionTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	exp := time.Now().Add(ttl)
	a.sessions[sessionToken] = restSession{
		Token:       sessionToken,
		Subject:     subj,
		Fingerprint: normalizedFingerprint,
		ExpiresAt:   exp,
		Generation:  generation,
	}
	a.mu.Unlock()

	writeJSON(w, http.StatusOK, authVerifyResponse{
		OK:           true,
		SessionToken: sessionToken,
		ExpiresAt:    exp.Format(time.RFC3339),
		Subject:      subj,
		Fingerprint:  fp,
	})
}

func (a *Adaptador) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{
		OK:      true,
		Service: "GrxFirma REST local",
	})
}

func (a *Adaptador) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := a.loadUISettings()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settingsResponse{
			OK:       true,
			Settings: settings,
		})
	case http.MethodPost:
		var settings map[string]any
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			writeError(w, http.StatusBadRequest, "json de ajustes invalido")
			return
		}
		if err := ports.ValidarSinCredencialesProxyEnClaro(settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ports.EliminarCredencialSeguridadUIEnClaro(settings)
		if err := a.saveUISettings(settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settingsResponse{
			OK:       true,
			Settings: settings,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
	}
}

func (a *Adaptador) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Servicio == nil {
		writeJSON(w, http.StatusOK, serviceStatusResponse{
			OK: true,
			Status: serviceStatusPayload{
				Installed: false,
				Running:   false,
				Platform:  runtime.GOOS,
				Method:    "no-configurado",
			},
		})
		return
	}
	estado, err := a.Servicio.Estado(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, serviceStatusResponse{
			OK: true,
			Status: serviceStatusPayload{
				Installed: false,
				Running:   false,
				Platform:  runtime.GOOS,
				Method:    "error",
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, serviceStatusResponse{
		OK: true,
		Status: serviceStatusPayload{
			Installed: estado.Instalado,
			Running:   estado.Activo,
			Platform:  estado.Plataforma,
			Method:    estado.Metodo,
		},
	})
}

func (a *Adaptador) handlePDFPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	var req pdfPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de preview invalido")
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		writeJSON(w, http.StatusBadRequest, pdfPreviewResponse{OK: false, Error: "path es obligatorio"})
		return
	}
	if a.rejectPDFPreviewPathOps(w) {
		return
	}
	if a.PreviewPDF == nil {
		writeJSON(w, http.StatusNotImplemented, pdfPreviewResponse{OK: false, Error: "caso de uso de preview PDF no configurado"})
		return
	}
	if errMsg := validatePathSecurity(path); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, pdfPreviewResponse{OK: false, Error: "ruta no valida: " + errMsg})
		return
	}
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		writeJSON(w, http.StatusBadRequest, pdfPreviewResponse{OK: false, Error: "solo se soporta preview de PDF"})
		return
	}
	pdfData, err := a.readPDFPreviewPath(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, pdfPreviewResponse{OK: false, Error: "no se pudo leer el PDF de forma segura"})
		return
	}
	headerLimit := min(len(pdfData), 1024)
	if !bytes.Contains(pdfData[:headerLimit], []byte("%PDF-")) {
		writeJSON(w, http.StatusBadRequest, pdfPreviewResponse{OK: false, Error: "el fichero no contiene una cabecera PDF valida"})
		return
	}

	previewDir, err := os.MkdirTemp("", "grxfirma-rest-preview-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, pdfPreviewResponse{OK: false, Error: "no se pudo preparar el PDF para previsualizacion"})
		return
	}
	defer os.RemoveAll(previewDir)
	previewPath := filepath.Join(previewDir, "input.pdf")
	if err := securefile.WriteFileAtomic(previewPath, pdfData, 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, pdfPreviewResponse{OK: false, Error: "no se pudo preparar el PDF para previsualizacion"})
		return
	}

	result, err := a.PreviewPDF.Ejecutar(r.Context(), application.PdfPreviewCommand{
		Ruta:   previewPath,
		Pagina: req.Page,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, pdfPreviewResponse{OK: false, Error: err.Error()})
		return
	}
	if strings.TrimSpace(result.DataB64) == "" {
		writeJSON(w, http.StatusInternalServerError, pdfPreviewResponse{OK: false, Error: "el renderer PDF no genero una imagen"})
		return
	}
	writeJSON(w, http.StatusOK, pdfPreviewResponse{
		OK:           true,
		ImageBase64:  result.DataB64,
		CurrentPage:  result.PaginaActual,
		TotalPages:   result.TotalPaginas,
		WidthPoints:  result.Ancho,
		HeightPoints: result.Alto,
		Data:         result.DataB64,
		Width:        result.Ancho,
		Height:       result.Alto,
	})
}

func (a *Adaptador) handleCertificatesImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	var req importCertificateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de importacion invalido")
		return
	}
	rawB64 := strings.TrimSpace(req.P12B64)
	if rawB64 == "" {
		writeJSON(w, http.StatusBadRequest, importCertificateResponse{OK: false, Error: "p12B64 es obligatorio"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, importCertificateResponse{OK: false, Error: "p12B64 invalido"})
		return
	}
	defer secmem.Zeroize(data)
	if len(data) > 10*1024*1024 {
		writeJSON(w, http.StatusRequestEntityTooLarge, importCertificateResponse{OK: false, Error: "fichero de certificado demasiado grande"})
		return
	}
	identidad, err := pkcs12importer.New().ImportIdentity(r.Context(), data, req.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, importCertificateResponse{OK: false, Error: err.Error()})
		return
	}
	destinoDir, err := a.directorioP12()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, importCertificateResponse{OK: false, Error: err.Error()})
		return
	}
	if err := os.MkdirAll(destinoDir, 0o700); err != nil {
		writeJSON(w, http.StatusBadRequest, importCertificateResponse{OK: false, Error: "no se pudo crear el directorio pkcs12"})
		return
	}
	destino := filepath.Join(destinoDir, nombreDestinoP12Importado(identidad.Reference))
	if err := securefile.WriteFileAtomic(destino, data, 0o600); err != nil {
		writeJSON(w, http.StatusBadRequest, importCertificateResponse{OK: false, Error: "no se pudo guardar el certificado importado"})
		return
	}
	writeJSON(w, http.StatusOK, importCertificateResponse{
		OK:          true,
		Destination: destino,
		Subject:     identidad.Reference.Subject,
		Fingerprint: identidad.Reference.Fingerprint,
	})
}

func (a *Adaptador) handleCertificateValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok":    false,
			"error": a.t("Método no permitido"),
		})
		return
	}
	if a.Catalogo == nil || a.Claves == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("El backend no está preparado para validar el certificado."),
		})
		return
	}

	var req certificateValidationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("Petición JSON inválida."),
		})
		return
	}
	certID := strings.TrimSpace(req.CertificateID)
	if certID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("Debe indicar el certificado a comprobar."),
		})
		return
	}

	ref, err := a.findCertificateRefByID(r.Context(), certID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if ref.ID == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"ok":    false,
			"error": a.t("No se encontró el certificado indicado."),
		})
		return
	}

	key, err := a.Claves.KeyFor(r.Context(), ref)
	defer ports.CloseSigningKey(key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("No se pudo acceder a la cadena del certificado: %s", err.Error()),
		})
		return
	}
	roots, err := systemCertPoolFunc()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": a.t("No se pudo acceder al almacén de confianza del sistema."),
		})
		return
	}
	if roots == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": a.t("El almacén de confianza del sistema está vacío."),
		})
		return
	}

	result, err := validateCertificateChainAt(certID, key.CertificateChainDER(), roots, time.Now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("No se pudo validar la cadena del certificado: %s", err.Error()),
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validateCertificateChainAt(certificateID string, chainDER [][]byte, roots *x509.CertPool, now time.Time) (certificateValidationResponse, error) {
	const (
		maxCertificateChainLength = 16
		maxCertificateDERBytes    = 2 * 1024 * 1024
	)
	if len(chainDER) == 0 {
		return certificateValidationResponse{}, errors.New("cadena X.509 vacía")
	}
	if len(chainDER) > maxCertificateChainLength {
		return certificateValidationResponse{}, errors.New("cadena X.509 demasiado larga")
	}
	if roots == nil {
		return certificateValidationResponse{}, errors.New("almacén de confianza vacío")
	}
	certificates := make([]*x509.Certificate, 0, len(chainDER))
	for i, der := range chainDER {
		if len(der) == 0 || len(der) > maxCertificateDERBytes {
			return certificateValidationResponse{}, fmt.Errorf("certificado X.509 %d con tamaño inválido", i+1)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return certificateValidationResponse{}, fmt.Errorf("certificado X.509 %d inválido", i+1)
		}
		certificates = append(certificates, cert)
	}

	leaf := certificates[0]
	timeValid := !now.Before(leaf.NotBefore) && !now.After(leaf.NotAfter)
	digitalSignature := leaf.KeyUsage == 0 || leaf.KeyUsage&x509.KeyUsageDigitalSignature != 0
	intermediates := x509.NewCertPool()
	for _, cert := range certificates[1:] {
		intermediates.AddCert(cert)
	}
	chains, verifyErr := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	trusted := verifyErr == nil && len(chains) > 0
	verifiedChainDepth := 0
	if trusted {
		verifiedChainDepth = len(chains[0])
	}
	issues := make([]string, 0, 3)
	if !timeValid {
		if now.Before(leaf.NotBefore) {
			issues = append(issues, "not_yet_valid")
		} else {
			issues = append(issues, "expired")
		}
	}
	if !digitalSignature {
		issues = append(issues, "digital_signature_not_allowed")
	}
	if !trusted {
		issues = append(issues, "untrusted")
	}
	fingerprint := sha256.Sum256(leaf.Raw)
	response := certificateValidationResponse{
		OK:                    true,
		Valid:                 timeValid && digitalSignature && trusted,
		CertificateID:         strings.TrimSpace(certificateID),
		Subject:               leaf.Subject.String(),
		Issuer:                leaf.Issuer.String(),
		FingerprintSHA256:     hex.EncodeToString(fingerprint[:]),
		NotBefore:             leaf.NotBefore.Format(time.RFC3339),
		NotAfter:              leaf.NotAfter.Format(time.RFC3339),
		TimeValid:             timeValid,
		DigitalSignatureUsage: digitalSignature,
		Trusted:               trusted,
		ChainDepth:            verifiedChainDepth,
		PresentedChainDepth:   len(certificates),
		Issues:                issues,
	}
	return response, nil
}

func (a *Adaptador) handleCertificateOnlineCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok":    false,
			"error": a.t("Método no permitido"),
		})
		return
	}
	if a.Catalogo == nil || a.Claves == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("El backend no está preparado para comprobar online el certificado."),
		})
		return
	}

	var req certificateOnlineCheckRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("Petición JSON inválida."),
		})
		return
	}

	certID := strings.TrimSpace(req.CertificateID)
	if certID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("Debe indicar el certificado a comprobar."),
		})
		return
	}

	ref, err := a.findCertificateRefByID(r.Context(), certID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if ref.ID == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"ok":    false,
			"error": a.t("No se encontró el certificado indicado."),
		})
		return
	}

	key, err := a.Claves.KeyFor(r.Context(), ref)
	defer ports.CloseSigningKey(key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": a.t("No se pudo acceder a la cadena del certificado: %s", err.Error()),
		})
		return
	}
	result, err := signer.CheckCertificateOnlineRevocation(r.Context(), key.CertificateChainDER())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":    false,
			"error": a.t("No se pudo completar la comprobación online: %s", err.Error()),
		})
		return
	}

	writeJSON(w, http.StatusOK, certificateOnlineCheckResponse{
		OK:          true,
		Status:      string(result.Status),
		UserMessage: result.UserMessage,
		Reason:      result.Reason,
		Method:      result.Method,
		CheckedAt:   formatRESTOptionalTime(result.CheckedAt),
		RevokedAt:   formatRESTOptionalTime(result.RevokedAt),
		OCSPURL:     result.OCSPURL,
		CRLURL:      result.CRLURL,
	})
}

func (a *Adaptador) handleInstallPublicRoots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}

	a.tlsMu.Lock()
	defer a.tlsMu.Unlock()

	deps := a.tlsRESTDependencies()
	if !deps.managedTrustLifecycleSupported() {
		writeJSON(w, http.StatusOK, publicRootsResponse{
			OK:                    false,
			LocalCertificateState: "not_generated",
			SystemTrustState:      "not_supported",
			Error:                 "La plataforma no ofrece un ciclo reversible de confianza TLS local.",
		})
		return
	}

	tlsDir := a.directorioTLSRest()
	artifactPaths := tlsRESTManagedArtifactPaths(tlsDir)
	if err := validateTLSRESTCleanupTargets(tlsDir, artifactPaths); err != nil {
		writeJSON(w, http.StatusOK, publicRootsResponse{
			OK:                    false,
			LocalCertificateState: "not_generated",
			SystemTrustState:      "not_installed",
			Error:                 a.t("No se pudo preparar el certificado TLS local."),
		})
		return
	}

	certFile, keyFile, rootCertFile, source, err :=
		deps.ensureBrowserCompatibleCertificate(tlsDir, defaultCertPrefix)
	expectedCertFile := filepath.Join(tlsDir, defaultCertPrefix+".crt.pem")
	expectedKeyFile := filepath.Join(tlsDir, defaultCertPrefix+".key.pem")
	expectedRootCertFile := filepath.Join(
		tlsDir,
		defaultCertPrefix+"-root.crt.pem",
	)
	if err != nil ||
		source != "grxfirma-local-ca" ||
		filepath.Clean(certFile) != filepath.Clean(expectedCertFile) ||
		filepath.Clean(keyFile) != filepath.Clean(expectedKeyFile) ||
		filepath.Clean(rootCertFile) != filepath.Clean(expectedRootCertFile) {
		writeJSON(w, http.StatusOK, publicRootsResponse{
			OK:                    false,
			LocalCertificateState: "not_generated",
			SystemTrustState:      "not_installed",
			Error:                 a.t("No se pudo preparar el certificado TLS local."),
		})
		return
	}
	if err := validateTLSRESTCleanupTargets(tlsDir, artifactPaths); err != nil {
		writeJSON(w, http.StatusOK, publicRootsResponse{
			OK:                    false,
			LocalCertificateState: "not_generated",
			SystemTrustState:      "not_installed",
			Error:                 a.t("No se pudo preparar el certificado TLS local."),
		})
		return
	}

	err = deps.ensureManagedTrust(r.Context(), rootCertFile)
	if err == nil {
		writeJSON(w, http.StatusOK, publicRootsResponse{
			OK:                    true,
			LocalCertificateState: "generated",
			SystemTrustState:      "installed",
		})
		return
	}
	writeJSON(w, http.StatusOK, publicRootsResponse{
		OK:                    false,
		LocalCertificateState: "generated",
		SystemTrustState:      "not_installed",
		Error:                 a.t("No se pudo instalar la confianza TLS local."),
	})
}

func (a *Adaptador) handleTLSTrustStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	writeJSON(w, http.StatusOK, tlsTrustStatusResponse{
		OK:       true,
		TLSStore: a.tlsStoreDiagnostic(),
	})
}

func (a *Adaptador) handleDiagnosticsReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}

	certificateCount := 0
	canSignCount := 0
	if a.Catalogo != nil {
		if refs, err := a.Catalogo.List(r.Context()); err == nil {
			certificateCount = len(refs)
			ahora := time.Now()
			for _, ref := range refs {
				canSign := ref.HasSigningKey &&
					!ref.NotAfter.IsZero() &&
					!ref.IsExpired(ahora)
				if canSign {
					canSignCount++
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, diagnosticsReportResponse{
		OK:               true,
		CertificateCount: certificateCount,
		CanSignCount:     canSignCount,
		TLSStore:         a.tlsStoreDiagnostic(),
	})
}

func (a *Adaptador) handleProxySecretStoreStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	runtimeMode := a.runtimeProxyMode(r.Context())
	if a.ProxySecrets == nil {
		writeJSON(w, http.StatusOK, proxySecretStoreStatusResponse{
			OK:          true,
			Available:   false,
			Platform:    runtime.GOOS,
			Backend:     "none",
			Reason:      "proxy secret store no configurado",
			RuntimeMode: runtimeMode,
		})
		return
	}
	status, err := a.ProxySecrets.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proxySecretStoreStatusResponse{
		OK:          true,
		Available:   status.Available,
		Platform:    status.Platform,
		Backend:     status.Backend,
		Reason:      status.Reason,
		RuntimeMode: runtimeMode,
	})
}

func (a *Adaptador) runtimeProxyMode(ctx context.Context) string {
	doc, err := usersettings.CargarDocumentoCompat(ctx, a.ConfigDir)
	if err != nil {
		return "fail-closed"
	}
	cfg, err := proxysecretstore.ResolveRuntimeProxy(ctx, doc.Proxy, a.ProxySecrets)
	if err == nil {
		return classifyRuntimeProxyMode(cfg)
	}
	return "fail-closed"
}

func classifyRuntimeProxyMode(cfg proxysecretstore.RuntimeProxyConfig) string {
	if !cfg.Enabled {
		return "disabled"
	}
	typ := strings.ToLower(strings.TrimSpace(cfg.Type))
	switch typ {
	case "system":
		return "system"
	case "manual":
		if strings.TrimSpace(cfg.SecretID) != "" {
			return "manual-secure-store"
		}
		return "manual-no-secret"
	default:
		return "default-environment"
	}
}

func (a *Adaptador) handleTLSClearStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}

	a.tlsMu.Lock()
	defer a.tlsMu.Unlock()

	deps := a.tlsRESTDependencies()
	if !deps.managedTrustLifecycleSupported() {
		writeJSON(w, http.StatusOK, tlsClearStoreResponse{
			OK:      false,
			Message: "La plataforma no ofrece un ciclo reversible de confianza TLS local.",
		})
		return
	}

	tlsDir := a.directorioTLSRest()
	artifactPaths := tlsRESTManagedArtifactPaths(tlsDir)
	if err := validateTLSRESTCleanupTargets(tlsDir, artifactPaths); err != nil {
		writeJSON(w, http.StatusOK, tlsClearStoreResponse{
			OK:      false,
			Message: "No se pudo validar el almacén TLS local.",
		})
		return
	}
	unlock, err := acquireTLSLock(filepath.Join(tlsDir, "."+defaultCertPrefix+"-ca.lock"))
	if err != nil {
		writeJSON(w, http.StatusOK, tlsClearStoreResponse{
			OK:      false,
			Message: "No se pudo bloquear el almacén TLS local.",
		})
		return
	}
	defer unlock()
	if err := validateTLSRESTCleanupTargets(tlsDir, artifactPaths); err != nil {
		writeJSON(w, http.StatusOK, tlsClearStoreResponse{
			OK:      false,
			Message: "No se pudo validar el almacén TLS local.",
		})
		return
	}

	rootCertFile := filepath.Join(
		tlsDir,
		defaultCertPrefix+"-root.crt.pem",
	)
	if err := deps.removeManagedTrust(r.Context(), rootCertFile); err != nil {
		// No se borra ningún artefacto: el inventario es la prueba que permite
		// reintentar la retirada sin atribuirnos certificados ajenos.
		writeJSON(w, http.StatusOK, tlsClearStoreResponse{
			OK:      false,
			Message: "No se pudo retirar la confianza TLS local gestionada.",
		})
		return
	}

	removed, err := removeTLSRESTManagedArtifacts(artifactPaths)
	if err != nil {
		writeJSON(w, http.StatusOK, tlsClearStoreResponse{
			OK:      false,
			Removed: removed,
			Message: "La confianza se retiró, pero no se pudieron borrar todos los artefactos TLS locales.",
		})
		return
	}
	writeJSON(w, http.StatusOK, tlsClearStoreResponse{
		OK:      true,
		Removed: removed,
		Message: "Confianza y artefactos TLS locales gestionados retirados.",
	})
}

func (a *Adaptador) tlsRESTDependencies() tlsRESTDependencies {
	deps := defaultTLSRESTDependencies()
	if a.tlsDeps.ensureBrowserCompatibleCertificate != nil {
		deps.ensureBrowserCompatibleCertificate =
			a.tlsDeps.ensureBrowserCompatibleCertificate
	}
	if a.tlsDeps.ensureManagedTrust != nil {
		deps.ensureManagedTrust = a.tlsDeps.ensureManagedTrust
	}
	if a.tlsDeps.removeManagedTrust != nil {
		deps.removeManagedTrust = a.tlsDeps.removeManagedTrust
	}
	if a.tlsDeps.managedTrustLifecycleSupported != nil {
		deps.managedTrustLifecycleSupported =
			a.tlsDeps.managedTrustLifecycleSupported
	}
	return deps
}

func tlsRESTManagedArtifactPaths(tlsDir string) []string {
	rootCertFile := filepath.Join(
		tlsDir,
		defaultCertPrefix+"-root.crt.pem",
	)
	return []string{
		filepath.Join(tlsDir, defaultCertPrefix+".crt.pem"),
		filepath.Join(tlsDir, defaultCertPrefix+".key.pem"),
		rootCertFile,
		filepath.Join(tlsDir, defaultCertPrefix+"-root.key.pem"),
		rootCertFile + ".trustcache.json",
		rootCertFile + ".grxfirma-trust.json",
		rootCertFile + ".grxfirma-nss-trust.json",
	}
}

func validateTLSRESTCleanupTargets(
	tlsDir string,
	artifactPaths []string,
) error {
	cleanDir := filepath.Clean(tlsDir)
	if strings.TrimSpace(tlsDir) == "" ||
		cleanDir == "." ||
		filepath.Base(cleanDir) != "tls" ||
		filepath.Dir(cleanDir) == cleanDir {
		return errors.New("directorio TLS local no válido")
	}

	seen := make(map[string]struct{}, len(artifactPaths))
	for _, path := range artifactPaths {
		cleanPath := filepath.Clean(path)
		if filepath.Dir(cleanPath) != cleanDir {
			return errors.New("un artefacto TLS queda fuera del almacén local")
		}
		if _, exists := seen[cleanPath]; exists {
			return errors.New("lista de artefactos TLS duplicada")
		}
		seen[cleanPath] = struct{}{}
	}

	cleanParent := filepath.Dir(cleanDir)
	if filepath.Dir(cleanParent) == cleanParent {
		return errors.New("directorio de configuración TLS demasiado amplio")
	}
	parentInfo, err := os.Lstat(cleanParent)
	if err == nil {
		if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
			return errors.New("el directorio de configuración TLS no es seguro")
		}
		parentHandle, openErr := securefile.OpenDir(cleanParent)
		if openErr != nil {
			return openErr
		}
		if closeErr := parentHandle.Close(); closeErr != nil {
			return closeErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	info, err := os.Lstat(cleanDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("el almacén TLS local no es un directorio seguro")
	}
	handle, err := securefile.OpenDir(cleanDir)
	if err != nil {
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}

	for _, path := range artifactPaths {
		cleanPath := filepath.Clean(path)
		info, err := os.Lstat(cleanPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("un artefacto TLS gestionado no es un fichero regular")
		}
		file, err := securefile.OpenRead(cleanPath)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func removeTLSRESTManagedArtifacts(artifactPaths []string) (int, error) {
	removed := 0
	for _, path := range artifactPaths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		if !info.Mode().IsRegular() {
			return removed, errors.New(
				"un artefacto TLS gestionado no es un fichero regular",
			)
		}
		file, err := securefile.OpenRead(path)
		if err != nil {
			return removed, err
		}
		if err := file.Close(); err != nil {
			return removed, err
		}
		if err := os.Remove(path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (a *Adaptador) handleServiceInstall(w http.ResponseWriter, r *http.Request) {
	a.handleServiceAction(w, r, "install")
}

func (a *Adaptador) handleServiceUninstall(w http.ResponseWriter, r *http.Request) {
	a.handleServiceAction(w, r, "uninstall")
}

func (a *Adaptador) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	a.handleServiceAction(w, r, "start")
}

func (a *Adaptador) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	a.handleServiceAction(w, r, "stop")
}

func (a *Adaptador) handleServiceAction(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Servicio == nil {
		writeJSON(w, http.StatusOK, serviceActionResponse{
			OK:      false,
			Error:   a.t("error.servicio_no_configurado"),
			Message: a.t("error.servicio_no_configurado"),
		})
		return
	}

	var params struct {
		IpcSocket string `json:"ipcSocket"`
	}
	if r.Body != nil {
		var err error
		decErr := json.NewDecoder(r.Body).Decode(&params)
		if decErr != nil && !errors.Is(decErr, io.EOF) {
			err = decErr
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "json de servicio invalido")
			return
		}
	}

	var callErr error
	var msg string
	switch action {
	case "install":
		socketPath := strings.TrimSpace(params.IpcSocket)
		if socketPath == "" {
			socketPath = a.defaultServiceSocketPath()
		}
		callErr = a.Servicio.Instalar(r.Context(), socketPath)
		msg = a.t("info.servicio_instalado")
	case "uninstall":
		callErr = a.Servicio.Desinstalar(r.Context())
		msg = a.t("info.servicio_desinstalado")
	case "start":
		callErr = a.Servicio.Iniciar(r.Context())
		msg = a.t("info.servicio_iniciado")
	case "stop":
		callErr = a.Servicio.Detener(r.Context())
		msg = a.t("info.servicio_detenido")
	default:
		writeError(w, http.StatusBadRequest, "accion de servicio no soportada")
		return
	}
	if callErr != nil {
		writeJSON(w, http.StatusOK, serviceActionResponse{
			OK:      false,
			Error:   callErr.Error(),
			Message: callErr.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, serviceActionResponse{
		Message: msg,
		OK:      true,
	})
}

func (a *Adaptador) handleCertificates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if a.Catalogo == nil {
		writeError(w, http.StatusNotImplemented, "catalogo de certificados no configurado")
		return
	}

	refs, err := a.Catalogo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	entries := make([]certificateEntryResponse, 0, len(refs))
	ahora := time.Now()
	for _, ref := range refs {
		// El catálogo publica esta capacidad al enumerar la identidad. No se
		// abre la clave durante un listado porque un token podría pedir PIN.
		vigenciaDesconocida := ref.NotAfter.IsZero()
		caducado := !vigenciaDesconocida && ref.IsExpired(ahora)
		canSign := ref.HasSigningKey && !vigenciaDesconocida && !caducado
		status := "No utilizable"
		if caducado {
			status = "Caducado"
		} else if vigenciaDesconocida {
			status = "Vigencia desconocida"
		} else if canSign {
			status = "Válido"
		}
		entries = append(entries, certificateEntryResponse{
			ID:           ref.ID,
			SubjectName:  ref.Subject,
			IssuerName:   ref.Issuer,
			ValidTo:      ref.NotAfter.Format(time.RFC3339),
			Fingerprint:  ref.Fingerprint,
			SerialNumber: "",
			Status:       status,
			CanSign:      canSign,
		})
	}

	writeJSON(w, http.StatusOK, certificatesResponse{
		OK:           true,
		Certificates: entries,
	})
}

func (a *Adaptador) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	enabled, _ := a.certificateAuthSnapshot()
	identidadHabilitada := a.PrepararIdentidad != nil && a.ConfirmarIdentidad != nil && a.authenticationConfigured()
	writeJSON(w, http.StatusOK, openAPIDocument(enabled, identidadHabilitada))
}

func (a *Adaptador) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	locale := a.requestLocale(r)
	writeHTML(w, http.StatusOK, trustedHTMLResponse(consoleIndexPage(locale, a.tForLocale(locale))))
}

func (a *Adaptador) handleSigner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if r.URL.Path != "/signer" && r.URL.Path != "/firmador" {
		http.NotFound(w, r)
		return
	}
	locale := a.requestLocale(r)
	writeHTML(w, http.StatusOK, trustedHTMLResponse(consoleSignerPage(locale, a.tForLocale(locale))))
}

func (a *Adaptador) handleValidator(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	if r.URL.Path != "/validator" && r.URL.Path != "/validador" {
		http.NotFound(w, r)
		return
	}
	locale := a.requestLocale(r)
	writeHTML(w, http.StatusOK, trustedHTMLResponse(consoleValidatorPage(locale, a.tForLocale(locale))))
}

// rejectPathOps devuelve true y escribe 403 si las operaciones de sistema de
// ficheros (inputPath/outputPath) no están habilitadas en este adaptador.
func (a *Adaptador) rejectPathOps(w http.ResponseWriter) bool {
	if a != nil && a.AllowFileSystemPaths {
		return false
	}
	writeError(w, http.StatusForbidden, "operaciones con rutas de fichero deshabilitadas; use content_base64")
	return true
}

func (a *Adaptador) rejectPDFPreviewPathOps(w http.ResponseWriter) bool {
	if a != nil && (a.AllowFileSystemPaths || a.AllowPDFPreviewPaths) {
		return false
	}
	writeError(w, http.StatusForbidden, "operaciones con rutas de fichero deshabilitadas")
	return true
}

func (a *Adaptador) readPathFile(path string) ([]byte, error) {
	limit := a.MaxBodyBytes
	if limit <= 0 {
		limit = 100 * 1024 * 1024
	}
	return securefile.ReadFileLimit(path, limit)
}

func (a *Adaptador) readPDFPreviewPath(path string) ([]byte, error) {
	limit := int64(maxPDFPreviewInputBytes)
	if a != nil && a.MaxBodyBytes > 0 && a.MaxBodyBytes < limit {
		limit = a.MaxBodyBytes
	}
	return securefile.ReadFileLimit(path, limit)
}

func (a *Adaptador) prepareSignOptions(options map[string]string) (map[string]string, error) {
	out := copiarOpcionesFirma(options)
	var imagePath string
	for key, value := range out {
		if strings.EqualFold(strings.TrimSpace(key), "visibleSealImagePath") {
			imagePath = strings.TrimSpace(value)
			delete(out, key)
		}
	}
	if imagePath == "" {
		return out, nil
	}
	if !a.AllowFileSystemPaths {
		return nil, errSignOptionPathDisabled
	}
	if errMsg := validatePathSecurity(imagePath); errMsg != "" {
		return nil, errors.New("ruta de imagen de sello no válida: " + errMsg)
	}
	data, err := securefile.ReadFileLimit(imagePath, maxVisibleSealImageBytes)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la imagen de sello: %w", err)
	}
	out["visibleSealImageBase64"] = base64.StdEncoding.EncodeToString(data)
	return out, nil
}

func (a *Adaptador) prepareSignOptionsWithDefaults(ctx context.Context, formato string, options map[string]string) (map[string]string, error) {
	explicitas, err := a.prepareSignOptions(options)
	if err != nil {
		return nil, err
	}
	if !ports.NecesitaOpcionesFirmaPredeterminadas(formato, explicitas) {
		return explicitas, nil
	}
	preferencias, err := a.loadSigningPreferences(ctx)
	if err != nil {
		// Un default de preferencias corrupto no debe bloquear la firma. PAdES
		// cae al subfiltro ETSI seguro del motor.
		return explicitas, nil
	}
	return ports.AplicarOpcionesFirmaPredeterminadas(preferencias, formato, explicitas), nil
}

func (a *Adaptador) loadSigningPreferences(ctx context.Context) (ports.DocumentoConfiguracionUsuario, error) {
	if a == nil || strings.TrimSpace(a.ConfigDir) == "" {
		return ports.DocumentoConfiguracionUsuario{}, nil
	}
	doc, err := usersettings.CargarDocumentoCompat(ctx, a.ConfigDir)
	if err != nil {
		return ports.DocumentoConfiguracionUsuario{}, errors.New("no se pudieron cargar los ajustes de firma")
	}
	return doc, nil
}

func mergeSignOptions(base, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	merged := copiarOpcionesFirma(base)
	if merged == nil {
		merged = make(map[string]string, len(overrides))
	}
	for overrideKey, value := range overrides {
		for existingKey := range merged {
			if strings.EqualFold(strings.TrimSpace(existingKey), strings.TrimSpace(overrideKey)) {
				delete(merged, existingKey)
			}
		}
		merged[overrideKey] = value
	}
	return merged
}

func writeSignOptionsError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, errSignOptionPathDisabled) {
		status = http.StatusForbidden
	}
	writeError(w, status, err.Error())
}

// validatePathSecurity valida que una ruta es segura para operaciones del sistema de ficheros.
// Rechaza intentos de path traversal, acceso a directorios críticos del sistema, etc.
// Retorna "" si la ruta es válida, o un mensaje de error si no.
// (H-01: CWE-22 / CWE-73 Path Traversal Mitigation)
func validatePathSecurity(inputPath string) string {
	if strings.TrimSpace(inputPath) == "" {
		return "ruta vacia"
	}

	// Normalizar la ruta y resolver symlinks
	absPath, err := filepath.Abs(strings.TrimSpace(inputPath))
	if err != nil {
		return fmt.Sprintf("ruta invalida: %v", err)
	}

	// Evaluar symlinks reales. Esta comprobación reduce la superficie, aunque la
	// apertura final debe seguir evitando enlaces para cerrar carreras TOCTOU.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// Si el archivo no existe, aún es válido si la ruta padre sí lo es
		parent := filepath.Dir(absPath)
		realParent, err2 := filepath.EvalSymlinks(parent)
		if err2 != nil {
			return fmt.Sprintf("ruta inaccesible: %v", err2)
		}
		// Usar la ruta normalizada del padre + el fichero
		realPath = filepath.Join(realParent, filepath.Base(absPath))
	}

	// Detectar path traversal: si contiene ".." después de normalización
	if strings.Contains(realPath, "..") {
		return "ruta con path traversal detectado"
	}

	// Lista de directorios prohibidos por defecto (sistema crítico)
	forbiddenPrefixes := []string{
		"/etc",
		"/sys",
		"/proc",
		"/root",
		"/dev",
		"/boot",
		"/bin",
		"/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/lib",
		"/lib64",
	}

	// En Windows, prohibir acceso a System32, Windows, etc.
	if runtime.GOOS == "windows" {
		forbiddenPrefixes = append(forbiddenPrefixes,
			"C:\\Windows",
			"C:\\Program Files",
			"C:\\Program Files (x86)",
			"C:\\ProgramData",
			"C:\\System Volume Information",
		)
	}

	for _, prefix := range forbiddenPrefixes {
		normalizedPrefix := filepath.FromSlash(prefix)
		if pathIsWithin(realPath, canonicalSecurityRoot(normalizedPrefix)) {
			return fmt.Sprintf("acceso prohibido a directorio del sistema: %s", prefix)
		}
	}

	// También prohibir acceso a directorios sensibles del usuario
	homeDir, err := os.UserHomeDir()
	if err == nil {
		sensitiveUserDirs := []string{
			filepath.Join(homeDir, ".ssh"),
			filepath.Join(homeDir, ".gnupg"),
			filepath.Join(homeDir, ".git"),
			filepath.Join(homeDir, ".aws"),
			appdirs.Config(homeDir),
			appdirs.Data(homeDir),
		}

		for _, sensitiveDir := range sensitiveUserDirs {
			if pathIsWithin(realPath, canonicalSecurityRoot(sensitiveDir)) {
				return fmt.Sprintf("acceso prohibido a directorio sensible: %s", sensitiveDir)
			}
		}
	}

	return ""
}

// canonicalSecurityRoot resuelve los alias de una raíz de seguridad incluso
// cuando su último tramo todavía no existe. Evita comparar una ruta candidata
// canónica (por ejemplo /private/etc en macOS) con un alias sin resolver (/etc).
func canonicalSecurityRoot(root string) string {
	cleanRoot := filepath.Clean(root)
	current := cleanRoot
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return cleanRoot
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathIsWithin(candidate, directory string) bool {
	cleanCandidate := filepath.Clean(candidate)
	cleanDirectory := filepath.Clean(directory)
	if runtime.GOOS == "windows" {
		cleanCandidate = strings.ToLower(cleanCandidate)
		cleanDirectory = strings.ToLower(cleanDirectory)
	}
	return cleanCandidate == cleanDirectory ||
		strings.HasPrefix(cleanCandidate, cleanDirectory+string(filepath.Separator))
}

func (a *Adaptador) authorize(next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticationConfigured() {
			next.ServeHTTP(w, r)
			return
		}
		if !a.authOK(r) {
			writeError(w, http.StatusUnauthorized, "autorizacion requerida")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Adaptador) authOK(r *http.Request) bool {
	a.cleanupExpiredAuthState()
	raw := authHeaderToken(r)
	if raw == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Comparacion en tiempo constante para evitar oraculos temporales (CWE-208).
	if expected := strings.TrimSpace(a.BearerToken); expected != "" {
		if subtle.ConstantTimeCompare([]byte(raw), []byte(expected)) == 1 {
			return true
		}
	}
	if !a.CertificateAuthEnabled {
		return false
	}
	sess, ok := a.sessions[raw]
	if !ok {
		return false
	}
	if time.Now().After(sess.ExpiresAt) ||
		sess.Generation != a.authGeneration {
		delete(a.sessions, raw)
		return false
	}
	if _, allowed := a.AllowedCerts[strings.ToLower(sess.Fingerprint)]; !allowed {
		delete(a.sessions, raw)
		return false
	}
	return true
}

func (a *Adaptador) authenticationConfigured() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.TrimSpace(a.BearerToken) != "" || a.CertificateAuthEnabled
}

func (a *Adaptador) certificateAuthSnapshot() (bool, uint64) {
	if a == nil {
		return false, 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.CertificateAuthEnabled, a.authGeneration
}

func authHeaderToken(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		raw = strings.TrimSpace(raw[len("Bearer "):])
	}
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("X-API-Token"))
	}
	return raw
}

func (a *Adaptador) cleanupExpiredAuthState() {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, ch := range a.challenges {
		if now.After(ch.ExpiresAt) {
			delete(a.challenges, k)
		}
	}
	for k, sess := range a.sessions {
		if now.After(sess.ExpiresAt) {
			delete(a.sessions, k)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// trustedHTMLResponse delimita contenido generado exclusivamente por las
// plantillas y los catalogos de traduccion incluidos en el binario.
type trustedHTMLResponse string

func writeHTML(w http.ResponseWriter, status int, body trustedHTMLResponse) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; connect-src 'self'; img-src 'self' data: blob:; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=(), usb=()")
	w.WriteHeader(status)
	// #nosec G705 -- los dos llamadores construyen body desde plantillas y catalogos
	// del binario; el locale de la peticion se normaliza y nunca se interpola sin escape.
	_, _ = w.Write([]byte(body))
}

func (a *Adaptador) t(id string, args ...any) string {
	if a != nil && a.Loc != nil {
		translated := a.Loc.T(id, args...)
		if len(args) > 0 && strings.Contains(translated, "%") {
			return fmt.Sprintf(translated, args...)
		}
		return translated
	}
	if len(args) == 0 {
		return id
	}
	return fmt.Sprintf(id, args...)
}

func (a *Adaptador) locale() string {
	type localeAware interface {
		Locale() string
	}
	if a != nil && a.Loc != nil {
		if l, ok := a.Loc.(localeAware); ok {
			locale := strings.TrimSpace(strings.ToLower(l.Locale()))
			if locale != "" {
				return locale
			}
		}
	}
	return "es"
}

func (a *Adaptador) requestLocale(r *http.Request) string {
	if r != nil {
		if requested := strings.TrimSpace(r.URL.Query().Get("lang")); requested != "" {
			return localizador.Para(requested).Locale()
		}
	}
	return a.locale()
}

func (a *Adaptador) tForLocale(locale string) func(string, ...any) string {
	normalized := localizador.Para(locale).Locale()
	if a == nil || a.Loc == nil {
		loc := localizador.Para(normalized)
		return func(id string, args ...any) string {
			return loc.T(id, args...)
		}
	}
	if normalized == a.locale() {
		return a.t
	}
	loc := localizador.Para(normalized)
	return func(id string, args ...any) string {
		return loc.T(id, args...)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	diag := application.BuildGuidedDiagnostic("", message)
	writeJSON(w, status, errorResponse{
		Error: message,
		Diagnostic: &guidedDiagnosticResponse{
			Category:               string(diag.Category),
			UserMessage:            diag.UserMessage,
			ExpertMessage:          diag.ExpertMessage,
			LikelyOwner:            diag.LikelyOwner,
			ResponsibilityMessage:  diag.ResponsibilityMessage,
			SuggestedAction:        diag.SuggestedAction,
			UserCanResolveDirectly: diag.UserCanResolveDirectly,
		},
	})
}

func inferOriginalMIMEType(signedMIME string) string {
	signedMIME = strings.ToLower(strings.TrimSpace(signedMIME))
	switch signedMIME {
	case "application/pkcs7-signature", "application/cms", "application/pkcs7-mime":
		return "application/octet-stream"
	default:
		return signedMIME
	}
}

func (a *Adaptador) resolveCertificateIDFromRequest(ctx context.Context, req signRequest) (string, error) {
	if id := strings.TrimSpace(req.CertificateID); id != "" {
		return id, nil
	}
	if req.CertificateIx == nil {
		return "", nil
	}
	if a.Catalogo == nil {
		return "", errors.New("catalogo de certificados no configurado")
	}
	refs, err := a.Catalogo.List(ctx)
	if err != nil {
		return "", err
	}
	idx := *req.CertificateIx
	if idx < 0 || idx >= len(refs) {
		return "", errors.New("certificateIndex fuera de rango")
	}
	return refs[idx].ID, nil
}

func (a *Adaptador) resolveProtectionCertificateID(ctx context.Context, req protectRequest) (string, error) {
	if id := strings.TrimSpace(req.CertificateID); id != "" {
		return id, nil
	}
	if req.CertificateIx == nil {
		return "", nil
	}
	if a.Catalogo == nil {
		return "", errors.New("catalogo de certificados no configurado")
	}
	refs, err := a.Catalogo.List(ctx)
	if err != nil {
		return "", err
	}
	idx := *req.CertificateIx
	if idx < 0 || idx >= len(refs) {
		return "", errors.New("certificateIndex fuera de rango")
	}
	return refs[idx].ID, nil
}

func addBoolOption(options map[string]string, key string, value *bool) {
	if value == nil {
		return
	}
	options[key] = strconv.FormatBool(*value)
}

func copiarOpcionesFirma(options map[string]string) map[string]string {
	if len(options) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(options))
	for k, v := range options {
		out[k] = v
	}
	return out
}

func (a *Adaptador) decodeBatchItemContent(item signBatchItemRequest) ([]byte, error) {
	inputPath := strings.TrimSpace(item.InputPath)
	if inputPath != "" {
		// H-01: Validar seguridad de ruta
		if errMsg := validatePathSecurity(inputPath); errMsg != "" {
			return nil, errors.New("ruta no valida: " + errMsg)
		}
		data, err := a.readPathFile(inputPath)
		if err != nil {
			return nil, errors.New("no se pudo leer inputPath del lote")
		}
		return data, nil
	}
	data, err := base64.StdEncoding.DecodeString(item.ContentBase64)
	if err != nil {
		return nil, errors.New("content_base64 del lote no es valido")
	}
	return data, nil
}

func normalizarAccionFirmaTexto(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "co-sign":
		return "cosign"
	case "counter-sign":
		return "countersign"
	default:
		return strings.TrimSpace(raw)
	}
}

func inferirFormatoSolicitud(raw, sourceName string) string {
	formato := strings.TrimSpace(raw)
	if formato == "" || strings.EqualFold(formato, "auto") {
		return inferirFormatoRuta(sourceName)
	}
	return formato
}

func valorBoolTexto(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "si", "sí", "overwrite":
		return true
	default:
		return false
	}
}

func inferirFormatoRuta(ruta string) string {
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".pdf":
		return string(domain.FormatPAdES)
	case ".asics":
		return "ASiC-XAdES"
	case ".odt", ".ods", ".odp", ".odg", ".odf":
		return "ODF"
	case ".docx", ".xlsx", ".pptx", ".ppsx":
		return "OOXML"
	case ".dsig", ".xmlsig":
		return "XMLdSig"
	case ".xml", ".xsig":
		return string(domain.FormatXAdES)
	default:
		return string(domain.FormatCAdES)
	}
}

func inferirTipoMIMERuta(ruta string) string {
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".pdf":
		return "application/pdf"
	case ".odt":
		return "application/vnd.oasis.opendocument.text"
	case ".ods":
		return "application/vnd.oasis.opendocument.spreadsheet"
	case ".odp":
		return "application/vnd.oasis.opendocument.presentation"
	case ".odg":
		return "application/vnd.oasis.opendocument.graphics"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".ppsx":
		return "application/vnd.openxmlformats-officedocument.presentationml.slideshow"
	case ".dsig", ".xmlsig":
		return "application/xmldsig+xml"
	case ".asics":
		return "application/vnd.etsi.asic-s+zip"
	case ".xml", ".xsig":
		return "application/xml"
	case ".json":
		return "application/json"
	case ".p7s", ".csig":
		return "application/pkcs7-signature"
	case ".enveloped", ".p7m":
		return domain.MIMETypeProtectedCMS
	case ".afp":
		return domain.MIMETypeProtectedEnvelope
	default:
		return "application/octet-stream"
	}
}

func normalizarPerfilProteccion(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "compat"
	}
	return raw
}

func normalizarDestinatariosProteccion(single string, many []string) []string {
	out := make([]string, 0, len(many)+1)
	seen := make(map[string]struct{}, len(many)+1)
	appendID := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	appendID(single)
	for _, id := range many {
		appendID(id)
	}
	return out
}

func normalizarOpcionesProteccion(in map[string]string, secretB64 string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	if raw, ok := out["container"]; ok {
		out["container"] = strings.ToLower(strings.TrimSpace(raw))
	}
	if trimmed := strings.TrimSpace(secretB64); trimmed != "" {
		out["secret_b64"] = trimmed
	}
	return out
}

func normalizarContenedorProteccionFirmada(options map[string]string) error {
	container := strings.ToLower(strings.TrimSpace(options["container"]))
	container = strings.ReplaceAll(container, "-", "")
	container = strings.ReplaceAll(container, "_", "")
	container = strings.ReplaceAll(container, " ", "")
	switch container {
	case "", "cmssignedandenveloped", "signedandenveloped", "signedandenvelopeddata":
		options["container"] = "signedandenvelopeddata"
		return nil
	case "cmsauthenveloped", "authenveloped", "authenvelopeddata", "authenticatedenvelopeddata":
		return errors.New("AuthEnvelopedData no esta soportado en protect-sign; solo se admite SignedAndEnvelopedData")
	case "cmsauthenticated", "cmsauthenticateddata", "authenticated", "authenticateddata":
		return errors.New("AuthenticatedData no esta soportado en protect-sign; solo se admite SignedAndEnvelopedData")
	case "cmscompressed", "cmscompresseddata", "compressed", "compresseddata":
		return errors.New("CompressedData no esta soportado en protect-sign; solo se admite SignedAndEnvelopedData")
	case "cmsencrypted", "cmsencrypteddata", "encrypted", "encrypteddata":
		return errors.New("EncryptedData no corresponde a protect-sign; use /protect con container=cms-encrypted")
	case "cms", "cmsenveloped", "cmsenvelopeddata", "enveloped", "envelopeddata":
		return errors.New("EnvelopedData no corresponde a protect-sign; use /protect con container=cms")
	default:
		return fmt.Errorf("contenedor CMS no soportado en protect-sign: %q", container)
	}
}

func normalizarMIMEProtegido(name, raw string) string {
	if strings.TrimSpace(raw) == "" {
		if inferred := inferirTipoMIMERuta(strings.TrimSpace(name)); inferred != "application/octet-stream" {
			return inferred
		}
		return domain.MIMETypeProtectedEnvelope
	}
	return raw
}

func construirSalidaRuta(rutaEntrada string, formato domain.SignatureFormat) string {
	base := strings.TrimSuffix(rutaEntrada, filepath.Ext(rutaEntrada))
	switch formato {
	case domain.FormatPAdES:
		return base + "_firmado.pdf"
	case domain.FormatXAdES:
		return base + ".xsig"
	case domain.SignatureFormat("XMLdSig"):
		return base + ".dsig"
	case domain.SignatureFormat("FacturaE"):
		return base + "_firmada.xml"
	case domain.SignatureFormat("ASiC-XAdES"):
		return base + ".asics"
	default:
		return base + ".csig"
	}
}

func construirSalidaProtegidaRuta(rutaEntrada, suggestedName string) string {
	if trimmed := strings.TrimSpace(suggestedName); trimmed != "" {
		return filepath.Join(filepath.Dir(rutaEntrada), filepath.Base(trimmed))
	}
	base := strings.TrimSpace(rutaEntrada)
	if base == "" {
		base = "documento"
	}
	return base + ".afp"
}

func construirSalidaDesprotegidaRuta(rutaEntrada, suggestedName string) string {
	if trimmed := strings.TrimSpace(suggestedName); trimmed != "" {
		return filepath.Join(filepath.Dir(rutaEntrada), filepath.Base(trimmed))
	}
	if strings.HasSuffix(strings.ToLower(rutaEntrada), ".afp") {
		return strings.TrimSuffix(rutaEntrada, filepath.Ext(rutaEntrada))
	}
	base := strings.TrimSuffix(rutaEntrada, filepath.Ext(rutaEntrada))
	if base == "" {
		base = rutaEntrada
	}
	return base + "_desprotegido"
}

func describirDestinatarioProteccion(recipient domain.ProtectionRecipient) (profile, algorithm string) {
	switch {
	case len(recipient.MLKEM768PublicKey) > 0 && len(recipient.X25519PublicKey) > 0:
		return "alto", "ML-KEM-768 + X25519"
	case len(recipient.RSAOAEP256PublicKeyDER) > 0:
		return "compat", "RSA-OAEP-SHA256"
	default:
		return "compat", "desconocido"
	}
}

func makeProtectionRecipientEntryResponse(recipient domain.ProtectionRecipient) protectionRecipientEntryResponse {
	profile, algorithm := describirDestinatarioProteccion(recipient)
	return protectionRecipientEntryResponse{
		ID:                          recipient.ID,
		Label:                       recipient.Label,
		Profile:                     profile,
		Algorithm:                   algorithm,
		AuthEnvelopedDataCompatible: supportsAuthEnvelopedData(recipient),
	}
}

func supportsAuthEnvelopedData(recipient domain.ProtectionRecipient) bool {
	if strings.TrimSpace(recipient.ID) == "" ||
		len(recipient.CertificateDER) == 0 ||
		len(recipient.RSAOAEP256PublicKeyDER) == 0 {
		return false
	}
	if len(recipient.CertificateDER) > 1<<20 || len(recipient.RSAOAEP256PublicKeyDER) > 64<<10 {
		return false
	}
	certificate, err := x509.ParseCertificate(recipient.CertificateDER)
	if err != nil || certificate.SerialNumber == nil || certificate.SerialNumber.Sign() <= 0 ||
		len(certificate.SerialNumber.Bytes()) > 20 || len(certificate.RawIssuer) == 0 || len(certificate.RawIssuer) > 16<<10 {
		return false
	}
	certificateKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok || certificateKey.N == nil || certificateKey.N.BitLen() < 2048 || certificateKey.N.BitLen() > 8192 {
		return false
	}
	if certificate.KeyUsage != 0 && certificate.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		return false
	}
	declaredKey, err := x509.ParsePKIXPublicKey(recipient.RSAOAEP256PublicKeyDER)
	if err != nil {
		return false
	}
	rsaDeclaredKey, ok := declaredKey.(*rsa.PublicKey)
	return ok && rsaDeclaredKey.N != nil &&
		rsaDeclaredKey.N.Cmp(certificateKey.N) == 0 &&
		rsaDeclaredKey.E == certificateKey.E
}

func (a *Adaptador) uiSettingsPath() (string, error) {
	if strings.TrimSpace(a.ConfigDir) == "" {
		return "", errors.New("configDir no configurado")
	}
	return filepath.Join(a.ConfigDir, "qt-settings.json"), nil
}

func (a *Adaptador) loadUISettings() (map[string]any, error) {
	path, err := a.uiSettingsPath()
	if err != nil {
		return nil, err
	}
	data, err := securefile.ReadFileLimit(path, 1024*1024)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, errors.New("fichero de ajustes QT inválido")
	}
	if settings == nil {
		settings = map[string]any{}
	}
	if ports.EliminarCredencialSeguridadUIEnClaro(settings) {
		if err := a.saveUISettings(settings); err != nil {
			return nil, fmt.Errorf("saneando credencial UI heredada de ajustes Qt: %w", err)
		}
	}
	return settings, nil
}

func (a *Adaptador) saveUISettings(settings map[string]any) error {
	path, err := a.uiSettingsPath()
	if err != nil {
		return err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	ports.EliminarCredencialSeguridadUIEnClaro(settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return securefile.WriteFileAtomic(path, data, 0o600)
}

func (a *Adaptador) defaultServiceSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "grxfirma_ipc.sock")
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".local", "run", "grxfirma_ipc.sock")
	}
	return filepath.Join(os.TempDir(), "grxfirma_ipc.sock")
}

func (a *Adaptador) directorioTLSRest() string {
	if strings.TrimSpace(a.ConfigDir) != "" {
		return filepath.Join(a.ConfigDir, "tls")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(os.TempDir(), "grxfirma", "tls")
	}
	return filepath.Join(appdirs.Config(home), "tls")
}

func (a *Adaptador) directorioP12() (string, error) {
	if strings.TrimSpace(a.ConfigDir) == "" {
		return "", errors.New("configDir no configurado")
	}
	if cfg, err := config.Load(a.ConfigDir); err == nil && strings.TrimSpace(cfg.DirectorioP12) != "" {
		return cfg.DirectorioP12, nil
	}
	return filepath.Join(a.ConfigDir, "pkcs12"), nil
}

func (a *Adaptador) tlsStoreDiagnostic() tlsStoreDiagnostic {
	dir := a.directorioTLSRest()
	resultado := tlsStoreDiagnostic{
		State:                 "not_created",
		LocalCertificateState: "not_generated",
		SystemTrustState:      "platform_dependent",
	}

	entries, err := os.ReadDir(dir)
	if err == nil {
		resultado.State = "empty"
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			resultado.ArtifactCount++
			nombre := strings.ToLower(entry.Name())
			switch {
			case strings.HasSuffix(nombre, ".key.pem") || strings.HasSuffix(nombre, ".key"):
				resultado.KeyCount++
			case strings.HasSuffix(nombre, ".crt.pem"),
				strings.HasSuffix(nombre, ".crt"),
				strings.HasSuffix(nombre, ".cer"),
				strings.HasSuffix(nombre, ".pem"):
				resultado.CertificateCount++
			}
		}
		if resultado.ArtifactCount > 0 {
			resultado.State = "available"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		resultado.State = "unavailable"
	}

	certFile := filepath.Join(dir, defaultCertPrefix+".crt.pem")
	if info, err := os.Stat(certFile); err == nil && info.Mode().IsRegular() {
		resultado.LocalCertificateState = "generated"
	}
	return resultado
}

func nombreDestinoP12Importado(ref domain.CertificateRef) string {
	fp := strings.ToLower(strings.TrimSpace(ref.Fingerprint))
	if fp == "" {
		return "certificado-importado.p12"
	}
	return "certificado-" + fp + ".p12"
}

var systemCertPoolFunc = x509.SystemCertPool

func parseAllowedFingerprints(csv string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(csv, ",") {
		v := strings.ToLower(strings.TrimSpace(part))
		v = strings.ReplaceAll(v, ":", "")
		if len(v) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(v); err != nil {
			continue
		}
		out[v] = struct{}{}
	}
	return out
}

func (a *Adaptador) findCertificateRefByID(ctx context.Context, certID string) (domain.CertificateRef, error) {
	if a == nil || a.Catalogo == nil {
		return domain.CertificateRef{}, nil
	}
	refs, err := a.Catalogo.List(ctx)
	if err != nil {
		return domain.CertificateRef{}, err
	}
	for _, ref := range refs {
		if ref.ID == certID {
			return ref, nil
		}
	}
	return domain.CertificateRef{}, nil
}

func formatRESTOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func newRandomTokenPair() (string, []byte, error) {
	idRaw := make([]byte, 16)
	nonceRaw := make([]byte, 32)
	if _, err := rand.Read(idRaw); err != nil {
		return "", nil, err
	}
	if _, err := rand.Read(nonceRaw); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(idRaw), nonceRaw, nil
}

func parseAuthCertificate(certPEM string, certB64 string) (*x509.Certificate, string, error) {
	var der []byte
	switch {
	case strings.TrimSpace(certPEM) != "":
		block, _ := pem.Decode([]byte(certPEM))
		if block == nil {
			return nil, "", errors.New("certificatePEM invalido")
		}
		der = block.Bytes
	case strings.TrimSpace(certB64) != "":
		raw, err := base64.StdEncoding.DecodeString(certB64)
		if err != nil {
			return nil, "", errors.New("certificateB64 invalido")
		}
		der = raw
	default:
		return nil, "", errors.New("debe proporcionar certificatePEM o certificateB64")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, "", errors.New("certificado X.509 invalido")
	}
	sum := sha256.Sum256(der)
	return cert, strings.ToLower(hex.EncodeToString(sum[:])), nil
}

func verifyChallengeSignature(cert *x509.Certificate, challenge []byte, signature []byte) error {
	digest := sha256.Sum256(challenge)
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], signature)
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, digest[:], signature) {
			return errors.New("firma ECDSA inválida")
		}
		return nil
	default:
		return errors.New("tipo de clave no soportado")
	}
}

func normalizeAuthVerifyRequestAliases(req *authVerifyRequest) {
	if strings.TrimSpace(req.ChallengeID) == "" {
		req.ChallengeID = req.ChallengeIDES
	}
	if strings.TrimSpace(req.SignatureB64) == "" {
		req.SignatureB64 = req.SignatureB64ES
	}
	if strings.TrimSpace(req.CertificatePEM) == "" {
		req.CertificatePEM = req.CertificatePEMES
	}
	if strings.TrimSpace(req.CertificateB64) == "" {
		req.CertificateB64 = req.CertificateB64ES
	}
}
