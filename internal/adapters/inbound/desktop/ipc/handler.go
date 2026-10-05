// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/updatecheck"
	"grxfirma/internal/adapters/outbound/desktop/clockdiagnostic"
	"grxfirma/internal/adapters/outbound/desktop/cscremota"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

// maxTamanoFichero es el limite de tamano de fichero que el IPC acepta para firmar o
// verificar. 100 MB es suficiente para documentos reales; evita ataques OOM.
const maxTamanoFichero = 100 * 1024 * 1024 // 100 MB

const (
	defaultIPCOperationTimeout = 30 * time.Second
	longIPCOperationTimeout    = 5 * time.Minute
	maxIPCPasswordBytes        = 4 * 1024
	maxIPCBatchDocuments       = 128
	maxIPCBatchTotalBytes      = 256 * 1024 * 1024
	tlsIPCCertificatePrefix    = "websocket-localhost"
)

// directoriosSistemaProhibidos lista prefijos de ruta que nunca deben leer o escribirse
// desde el IPC, independientemente de los permisos del proceso.
var directoriosSistemaProhibidos = []string{
	"/proc", "/sys", "/dev", "/run/udev", "/boot",
	"/root",                      // home del superusuario
	"/etc/shadow", "/etc/passwd", // ficheros criticos individuales
	"/etc/ssh",         // claves de host SSH
	"/etc/ssl/private", // claves TLS del sistema
	"/etc/master.passwd",
	// En macOS /etc y /var son enlaces a /private; EvalSymlinks devuelve esta forma.
	"/private/etc/passwd", "/private/etc/master.passwd", "/private/etc/ssh",
	"/private/etc/ssl/private", "/private/var/root",
}

type tlsIPCDependencies struct {
	ensureBrowserCompatibleCertificate func(
		dir string,
		prefix string,
	) (certFile, keyFile, rootCertFile, source string, err error)
	ensureManagedTrust func(context.Context, string) error
	removeManagedTrust func(context.Context, string) error
}

func defaultTLSIPCDependencies() tlsIPCDependencies {
	return tlsIPCDependencies{
		ensureBrowserCompatibleCertificate: resttls.EnsureBrowserCompatibleLocalhostCertificate,
		ensureManagedTrust:                 localtlstrust.EnsureManagedTrusted,
		removeManagedTrust:                 localtlstrust.RemoveManagedTrusted,
	}
}

// SignDocumentUseCase define el contrato minimo para firmar documentos.
type SignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

type MultiCoSignUseCase interface {
	Execute(ctx context.Context, cmd application.MultiCoSignCommand) (application.SignResult, error)
}

type metaFirmaLote struct {
	inputPath  string
	outputPath string
	format     string
	options    map[string]string
	content    []byte
}

// VerifySignatureUseCase define el contrato minimo para verificar firmas.
type VerifySignatureUseCase interface {
	Execute(ctx context.Context, cmd application.VerifyCommand) (application.VerifyResult, error)
}

type ProtectDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.ProtectCommand) (application.ProtectResult, error)
}

type ProtectAndSignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error)
}

type UnprotectDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.UnprotectCommand) (application.UnprotectResult, error)
}

type ProtectionRecipients interface {
	ports.ProtectionRecipientCatalog
	List(ctx context.Context) ([]domain.ProtectionRecipient, error)
}

type publicProtectionRecipients interface {
	ImportPublic(context.Context, []byte) (domain.ProtectionRecipient, error)
	RemovePublic(context.Context, string) error
}

type CreateHashUseCase interface {
	Execute(ctx context.Context, cmd application.CreateHashCommand) (application.CreateHashResult, error)
}

type CheckHashUseCase interface {
	Execute(ctx context.Context, cmd application.CheckHashCommand) (application.CheckHashResult, error)
}

type CreateDirectoryHashUseCase interface {
	Execute(ctx context.Context, cmd application.CreateDirectoryHashManifestCommand) (application.CreateDirectoryHashManifestResult, error)
}

type CheckDirectoryHashUseCase interface {
	Execute(ctx context.Context, cmd application.CheckDirectoryHashManifestCommand) (application.CheckDirectoryHashManifestResult, error)
}

type ClockDiagnosticProvider interface {
	Diagnose(ctx context.Context) clockdiagnostic.Report
}

type UpdateChecker interface {
	Comprobar(ctx context.Context, versionActual string) (updatecheck.Resultado, error)
}

// ProcessBatchUseCase define el contrato minimo para firmar lotes.
type ProcessBatchUseCase interface {
	Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error)
}

// PdfPreviewUseCase define el contrato minimo para renderizar paginas PDF.
type PdfPreviewUseCase interface {
	Ejecutar(ctx context.Context, cmd application.PdfPreviewCommand) (application.PdfPreviewResult, error)
}

// Manejador despacha las peticiones IPC a los casos de uso correspondientes.
// Mantiene una cache de la ultima lista de certificados para resolver por indice.
type Manejador struct {
	Catalogo              ports.CertificateCatalog
	Firmar                SignDocumentUseCase
	MultiCofirmar         MultiCoSignUseCase
	ProcesarLote          ProcessBatchUseCase
	Verificar             VerifySignatureUseCase
	Proteger              ProtectDocumentUseCase
	ProtegerFirmando      ProtectAndSignDocumentUseCase
	Desproteger           UnprotectDocumentUseCase
	Destinatarios         ProtectionRecipients
	CrearHash             CreateHashUseCase
	ComprobarHash         CheckHashUseCase
	CrearHashDir          CreateDirectoryHashUseCase
	ComprobarHashDir      CheckDirectoryHashUseCase
	InformeHashDir        ports.DirectoryHashReportCodec
	Preview               PdfPreviewUseCase
	Servicio              ports.GestorServicio
	Settings              ports.ConfiguracionUsuario
	TokenSettings         ports.LocalTokenSettings
	ProxySecrets          ports.ProxySecretStore
	Claves                ports.SigningKeyProvider
	Importador            ports.CertificateImporter
	CertificateAccess     *application.CertificateAccessUseCase
	TemporaryCertificates *application.TemporaryCertificateUseCase
	Loc                   ports.Localizador
	ConfigDir             string
	ClockDiagnostics      ClockDiagnosticProvider
	UpdateChecker         UpdateChecker
	CurrentVersion        string
	// CSC es la sesión de firma remota; nil si el motor no la ofrece.
	CSC               SesionCSC
	smartcardDetector smartcardDetector

	settingsMu             sync.Mutex
	certsMu                sync.RWMutex
	tlsDeps                tlsIPCDependencies
	localTLSStartupMu      sync.RWMutex
	localTLSStartupStatus  LocalTLSStartupStatus
	localTLSStartupRefresh func(context.Context) LocalTLSStartupStatus
	localTLSRefreshMu      sync.Mutex

	// ultimosCerts guarda la lista de certificados de la ultima consulta,
	// permitiendo que la accion "sign" resuelva por indice.
	ultimosCerts []domain.CertificateRef
}

// despacharConTimeout envuelve despachar con un contexto con timeout ajustado
// a la operacion: las operaciones de firma/verificacion obtienen 5 minutos,
// el resto 30 segundos.
func (m *Manejador) despacharConTimeout(parent context.Context, p peticion) respuesta {
	accion := strings.ToLower(strings.TrimSpace(p.Action))
	ctx, cancel := context.WithTimeout(parent, desktopIPCTimeoutDe(accion))
	defer cancel()
	return m.despachar(ctx, p)
}

func desktopIPCTimeoutDe(accion string) time.Duration {
	switch accion {
	case "sign", "sign_multicosign", "sign_batch", "protect_sign",
		"verify", "pdf_preview", "hash_create", "hash_check", "validate_invoice", "validate_eni", "generate_eni_document", "generate_eni_file",
		// Conectar espera a que la persona autorice en el navegador.
		"csc_connect":
		return longIPCOperationTimeout
	}
	return defaultIPCOperationTimeout
}

// despachar procesa una peticion y devuelve la respuesta serializada.
func (m *Manejador) despachar(ctx context.Context, p peticion) respuesta {
	accion := strings.ToLower(strings.TrimSpace(p.Action))
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: accion, Error: err.Error()}
	}
	var resp respuesta
	var peticionRemota *cscremota.Peticion
	if isRemoteSigningAction(accion) {
		var (
			liberar func()
			rechazo *respuesta
		)
		ctx, peticionRemota, liberar, rechazo = m.prepararFirmaRemota(ctx, accion, p.Params)
		defer liberar()
		if rechazo != nil {
			return normalizeIPCResponse(*rechazo)
		}
	}
	switch accion {
	case "csc_status", "csc_configure", "csc_connect", "csc_disconnect", "csc_send_otp":
		resp = m.handleCSC(ctx, accion, p.Params)
	case "hello":
		resp = respuesta{OK: true, Action: accion, Data: desktopIPCHello()}
	case "ping":
		resp = respuesta{OK: true, Action: accion}
	case "local_tls_startup_status":
		resp = respuesta{OK: true, Action: accion, Data: m.refreshLocalTLSStartup(ctx)}
	case "certificates", "getcertificates":
		resp = m.handleCertificados(ctx, accion)
	case "certificate_export_public":
		resp = m.handleCertificateExportPublic(ctx, p.Params)
	case "facturae_create":
		resp = handleFacturaeCreate(ctx, p.Params)
	case "validate_eni":
		resp = m.handleValidateENI(ctx, p.Params)
	case "validate_verifactu", "read_verifactu_qr", "query_verifactu_qr", "detect_verifactu":
		resp = m.handleVeriFactu(ctx, accion, p.Params)
	case "validate_invoice":
		resp = m.handleValidateInvoice(ctx, p.Params)
	case "generate_eni_document":
		resp = m.handleGenerateENIDocument(ctx, p.Params)
	case "generate_eni_file":
		resp = m.handleGenerateENIFile(ctx, p.Params)
	case "smartcard_status":
		detector := m.smartcardDetector
		if detector == nil {
			detector = detectSmartcards
		}
		resp = handleSmartcardStatus(ctx, detector)
	case "check_certificates":
		resp = m.handleCheckCertificados(ctx)
	case "check_updates":
		resp = m.handleCheckUpdates(ctx)
	case "sign":
		resp = m.handleFirma(ctx, p.Params)
	case "sign_multicosign":
		resp = m.handleFirmaMultiCofirma(ctx, p.Params)
	case "sign_batch":
		resp = m.handleFirmaLote(ctx, p.Params)
	case "verify":
		resp = m.handleVerificacion(ctx, p.Params)
	case "validate_certificate_online":
		resp = m.handleCertificateOnlineCheck(ctx, p.Params)
	case "protection_recipients":
		resp = m.handleProtectionRecipients(ctx)
	case "protection_recipient_import":
		resp = m.handleProtectionRecipientImport(ctx, p.Params)
	case "protection_recipient_remove":
		resp = m.handleProtectionRecipientRemove(ctx, p.Params)
	case "protect":
		resp = m.handleProtect(ctx, p.Params)
	case "protect_sign":
		resp = m.handleProtectSign(ctx, p.Params)
	case "unprotect":
		resp = m.handleUnprotect(ctx, p.Params)
	case "hash_create":
		resp = m.handleHashCreate(ctx, p.Params)
	case "hash_check":
		resp = m.handleHashCheck(ctx, p.Params)
	case "pdf_preview":
		resp = m.handlePdfPreview(ctx, p.Params)
	case "seal_preview":
		resp = m.handleVistaPreviaSello(ctx, p.Params)
	case "service_status":
		resp = m.handleServiceStatus(ctx)
	case "service_install":
		resp = m.handleServiceInstall(ctx, p.Params)
	case "service_uninstall":
		resp = m.handleServiceUninstall(ctx)
	case "service_start":
		resp = m.handleServiceStart(ctx)
	case "service_stop":
		resp = m.handleServiceStop(ctx)
	case "get_settings":
		resp = m.handleGetSettings(ctx)
	case "save_settings":
		resp = m.handleSaveSettings(ctx, p.Params)
	case "get_token_settings", "save_token_settings", "diagnose_token_settings":
		resp = m.handleLocalTokenSettings(ctx, accion, p.Params)
	case "proxy_secret_store_status":
		resp = m.handleProxySecretStoreStatus(ctx)
	case "proxy_secret_store":
		resp = m.handleProxySecretStore(ctx, p.Params)
	case "proxy_secret_delete":
		resp = m.handleProxySecretDelete(ctx)
	case "import_certificate":
		resp = m.handleImportCert(ctx, p.Params)
	case "certificate_access_options":
		resp = m.handleCertificateAccessOptions(ctx)
	case "open_certificate_manager":
		resp = m.handleOpenCertificateManager(ctx, p.Params)
	case "import_certificate_to_store":
		resp = m.handleImportCertificateToStore(ctx, p.Params)
	case "use_temporary_certificate":
		resp = m.handleUseTemporaryCertificate(ctx, p.Params)
	case "remove_temporary_certificate":
		resp = m.handleRemoveTemporaryCertificate(ctx, p.Params)
	case "clear_temporary_certificates":
		resp = m.handleClearTemporaryCertificates(ctx)
	case "tls_diagnostics":
		resp = m.handleTlsDiagnostics(ctx)
	case "clock_diagnostics":
		resp = m.handleClockDiagnostics(ctx)
	case "clear_tls_trust":
		resp = m.handleClearTlsTrust(ctx)
	case "export_diagnostic":
		resp = m.handleExportDiagnostic(ctx)
	case "install_public_roots":
		resp = m.handleInstallPublicRoots(ctx)
	default:
		resp = respuesta{
			OK:        false,
			Action:    accion,
			ErrorCode: "unsupported_action",
			Error:     m.t("error.accion_no_soportada", accion),
		}
	}
	if accion == "facturae_create" && !resp.OK && strings.HasPrefix(resp.Error, "facturae.error.") {
		resp.ErrorCode = resp.Error
		resp.Diagnostic = &resultadoDiagnosticoGuiado{
			Category: "app_local", FailureCode: resp.Error,
			UserMessage: m.t(resp.Error), ExpertMessage: resp.Error,
		}
	}
	resp = m.explicarFalloRemoto(resp, peticionRemota)
	return normalizeIPCResponse(resp)
}

func (m *Manejador) handleCheckUpdates(ctx context.Context) respuesta {
	const action = "check_updates"
	if m.UpdateChecker == nil {
		return respuesta{
			OK:     false,
			Action: action,
			Error:  m.t("No se puede comprobar la versión porque el servicio de actualizaciones no está configurado. Reinstale GrxFirma desde el repositorio oficial."),
		}
	}
	currentVersion := strings.TrimSpace(m.CurrentVersion)
	if currentVersion == "" {
		currentVersion = "dev"
	}
	result, err := m.UpdateChecker.Comprobar(ctx, currentVersion)
	if err != nil {
		code := updatecheck.FailureCode(err)
		userMessage := m.t(updatecheck.MessageKey(err))
		slog.WarnContext(ctx, "falló la consulta de versiones",
			"code", code, "error_type", fmt.Sprintf("%T", err))
		return respuesta{
			OK:        false,
			Action:    action,
			ErrorCode: updatecheck.ErrorCode(err),
			Error:     userMessage,
			Diagnostic: &resultadoDiagnosticoGuiado{
				Category: "remote_service", FailureCode: updatecheck.ErrorCode(err),
				UserMessage: userMessage, ExpertMessage: code,
			},
		}
	}
	if result.Estado == updatecheck.EstadoSinPublicaciones {
		result.Titulo = m.t("Sin versiones publicadas")
		result.Mensaje = m.t("Todavía no hay versiones publicadas en el canal oficial.")
	}
	return respuesta{OK: true, Action: action, Data: result}
}

func (m *Manejador) handleClockDiagnostics(
	ctx context.Context,
) respuesta {
	provider := m.ClockDiagnostics
	if provider == nil {
		provider = clockdiagnostic.New(nil)
	}
	return respuesta{
		OK:     true,
		Action: "clock_diagnostics",
		Data:   provider.Diagnose(ctx),
	}
}

func enrichIPCErrorDiagnostic(resp respuesta) respuesta {
	if resp.OK || strings.TrimSpace(resp.Error) == "" || resp.Diagnostic != nil {
		return resp
	}
	resp.Diagnostic = guidedDiagnosticResult(resp.Action, resp.Error)
	return resp
}

func guidedDiagnosticResult(action, message string) *resultadoDiagnosticoGuiado {
	diag := application.BuildGuidedDiagnostic(action, message)
	return &resultadoDiagnosticoGuiado{
		Category:               string(diag.Category),
		FailureCode:            diag.FailureCode,
		UserMessage:            diag.UserMessage,
		ExpertMessage:          diag.ExpertMessage,
		LikelyOwner:            diag.LikelyOwner,
		ResponsibilityMessage:  diag.ResponsibilityMessage,
		SuggestedAction:        diag.SuggestedAction,
		UserCanResolveDirectly: diag.UserCanResolveDirectly,
	}
}

// ---------------------------------------------------------------------------
// Acciones de certificados
// ---------------------------------------------------------------------------

func (m *Manejador) handleCertificados(ctx context.Context, accion string) respuesta {
	if m.Catalogo == nil {
		return respuesta{OK: false, Action: accion, Error: m.t("error.catalogo_no_configurado")}
	}
	certs, err := m.Catalogo.List(ctx)
	if err != nil {
		return respuesta{OK: false, Action: accion, Error: err.Error()}
	}
	m.setUltimosCerts(certs)
	return respuesta{OK: true, Action: accion, Data: m.certsAJSON(certs)}
}

// handleCertificateExportPublic solo usa el DER del catálogo. Nunca abre una
// SigningKey ni procesa un contenedor que pueda incluir material privado.
func (m *Manejador) handleCertificateExportPublic(ctx context.Context, raw json.RawMessage) respuesta {
	const action = "certificate_export_public"
	var params struct {
		CertificateID string `json:"certificateId"`
		OutputPath    string `json:"outputPath"`
		Format        string `json:"format"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || strings.TrimSpace(params.CertificateID) == "" {
		return respuesta{OK: false, Action: action, Error: "debe seleccionar un certificado del catálogo"}
	}
	if m.Catalogo == nil {
		return respuesta{OK: false, Action: action, Error: "catálogo de certificados no configurado"}
	}
	// Consultar de nuevo evita exportar una entrada obsoleta de la caché.
	refs, err := m.Catalogo.List(ctx)
	if err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	var der []byte
	for _, ref := range refs {
		if ref.ID == params.CertificateID {
			if !ref.HasLocalDecryptionKey {
				return certificateExportPublicRejected("Este certificado no servirá para que le protejan archivos con GrxFirma: su clave privada no está disponible para Desproteger RSA-OAEP. Importe una credencial P12/PFX propia con clave RSA compatible.")
			}
			der = ref.DER
			break
		}
	}
	if len(der) == 0 {
		return respuesta{OK: false, Action: action, Error: "certificado no encontrado o sin certificado X.509 público"}
	}
	recipient, err := protector.ValidatePublicRecipientCertificate(der)
	if err != nil {
		return certificateExportPublicRejected("Este certificado no servirá para que le protejan archivos: " + mensajeRechazoDestinatarioPublico(err))
	}
	format := strings.ToLower(strings.TrimSpace(params.Format))
	if format == "" {
		format = "der"
	}
	if format != "der" && format != "pem" {
		return respuesta{OK: false, Action: action, Error: "formato de certificado público no válido"}
	}
	outputPath := strings.TrimSpace(params.OutputPath)
	if outputPath != "" {
		if err := validarRutaEscritura(outputPath); err != nil {
			return respuesta{OK: false, Action: action, Error: err.Error()}
		}
		ext := strings.ToLower(filepath.Ext(outputPath))
		if (format == "der" && ext != ".cer") || (format == "pem" && ext != ".pem") {
			return respuesta{OK: false, Action: action, Error: "la extensión de salida debe ser .cer para DER o .pem para PEM"}
		}
		data := recipient.CertificateDER
		if format == "pem" {
			data = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: data})
		}
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: action, Error: err.Error()}
		}
		outputPath, err = filesystem.NuevoEscritorResultado(filesystem.PoliticaForzar).Escribir(outputPath, data)
		if err != nil {
			return respuesta{OK: false, Action: action, Error: err.Error()}
		}
	}
	return respuesta{OK: true, Action: action, Data: map[string]any{
		"certificateDerBase64": base64.StdEncoding.EncodeToString(recipient.CertificateDER),
		"outputPath":           outputPath, "format": format, "encryptionSuitable": true,
	}}
}

func certificateExportPublicRejected(message string) respuesta {
	return respuesta{OK: false, Action: "certificate_export_public", Error: message,
		Diagnostic: &resultadoDiagnosticoGuiado{
			Category: "app_local", UserMessage: message, ExpertMessage: message,
			SuggestedAction:        "Seleccione un certificado propio RSA vigente de al menos 2048 bits con keyEncipherment.",
			UserCanResolveDirectly: true,
		}}
}

func (m *Manejador) handleCheckCertificados(ctx context.Context) respuesta {
	if m.Catalogo == nil {
		return respuesta{OK: false, Action: "check_certificates", Error: m.t("error.catalogo_no_configurado")}
	}
	certs, err := m.Catalogo.List(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "check_certificates", Error: err.Error()}
	}
	m.setUltimosCerts(certs)
	items := m.certsAJSON(certs)
	fail := 0
	for _, c := range items {
		if c.Caducado || !c.CanSign {
			fail++
		}
	}
	return respuesta{OK: true, Action: "check_certificates", Data: resultadoCheckCerts{
		Certificates: items,
		OkCount:      len(items) - fail,
		FailCount:    fail,
	}}
}

func (m *Manejador) handleCertificateOnlineCheck(ctx context.Context, raw json.RawMessage) respuesta {
	var params paramsCertificateOnlineCheck
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("error.formato_invalido")}
	}
	certID := strings.TrimSpace(params.CertificateID)
	if certID == "" {
		return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("Debe indicar el certificado que quiere comprobar.")}
	}
	ref, ok, err := m.resolverCertRefPorID(ctx, certID)
	if err != nil {
		return respuesta{OK: false, Action: "validate_certificate_online", Error: err.Error()}
	}
	if !ok {
		return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("No se ha encontrado el certificado seleccionado en el catálogo actual.")}
	}
	// La comprobación de revocación solo necesita el certificado público. No
	// debe abrir la clave privada ni copiar la base NSS salvo que el catálogo
	// carezca del certificado y sea imprescindible recuperar su cadena.
	chain := make([][]byte, 0, 1+len(ref.ChainDER))
	chain = append(chain, ref.DER)
	chain = append(chain, ref.ChainDER...)
	if len(ref.DER) == 0 {
		if m.Claves == nil {
			return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("El catálogo no incluye el certificado público y no hay un almacén disponible para leerlo.")}
		}
		key, keyErr := m.Claves.KeyFor(ctx, ref)
		if keyErr != nil {
			ports.CloseSigningKey(key)
			return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("No se pudo leer el certificado seleccionado. Compruebe que sigue instalado y que los archivos de su almacén son accesibles.")}
		}
		defer ports.CloseSigningKey(key)
		chain = key.CertificateChainDER()
	} else {
		huella := sha256.Sum256(ref.DER)
		if len(ref.DER) > 1<<20 || (ref.Fingerprint != "" && !strings.EqualFold(hex.EncodeToString(huella[:]), ref.Fingerprint)) {
			return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("El certificado público del catálogo no coincide con la identidad seleccionada.")}
		}
	}
	result, err := signer.CheckCertificateOnlineRevocation(ctx, chain)
	if err != nil {
		return respuesta{OK: false, Action: "validate_certificate_online", Error: m.t("No se pudo comprobar online el estado del certificado: %s", err.Error())}
	}
	message := result.UserMessage
	if message == "" {
		message = m.t("Comprobación online del certificado finalizada.")
	}
	return respuesta{
		OK:     true,
		Action: "validate_certificate_online",
		Data: resultadoEstadoCertificadoOnline{
			Status:      string(result.Status),
			UserMessage: result.UserMessage,
			Reason:      result.Reason,
			Method:      result.Method,
			CheckedAt:   formatOptionalTime(result.CheckedAt),
			RevokedAt:   formatOptionalTime(result.RevokedAt),
			OCSPURL:     result.OCSPURL,
			CRLURL:      result.CRLURL,
		},
		Error: message,
	}
}

// ---------------------------------------------------------------------------
// Accion de firma
// ---------------------------------------------------------------------------

func (m *Manejador) handleFirma(ctx context.Context, raw json.RawMessage) respuesta {
	if m.Firmar == nil {
		return respuesta{OK: false, Action: "sign", Error: m.t("error.ipc_no_configurado")}
	}
	var params paramsFirma
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "sign", Error: m.t("error.formato_invalido")}
	}

	// Validar ruta de entrada: evita path traversal y acceso a ficheros de sistema.
	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}

	// Verificar tamano antes de leer (evita OOM).
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}

	contenido, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: "sign", Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}

	// Validar ruta de salida si se proporciona.
	if params.OutputPath != "" {
		if err := validarRutaEscritura(params.OutputPath); err != nil {
			return respuesta{OK: false, Action: "sign", Error: err.Error()}
		}
	}

	certID, err := m.resolverCertIDPreferido(ctx, params.CertificateID, params.CertificateIndex)
	if err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}
	nombre := filepath.Base(params.InputPath)
	mime := mimeDesdeExtension(params.InputPath)
	formato := params.Format
	if formato == "" {
		formato = inferirFormatoRuta(params.InputPath)
	}
	accionFirma := params.Action
	if accionFirma == "" {
		accionFirma = "sign"
	}
	params.Action = accionFirma
	opciones, err := construirOpcionesFirmaIPC(params, formato)
	if err != nil {
		return respuesta{OK: false, Action: "sign", Error: m.localizarErrorOpacidadLogoSello(err)}
	}
	opciones, err = m.aplicarOpcionesFirmaPredeterminadas(ctx, formato, opciones)
	if err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}

	cmd, err := application.NewSignCommand(nombre, contenido, mime, formato, accionFirma, certID, opciones)
	if err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}

	result, err := m.Firmar.Execute(ctx, cmd)
	if err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}

	if params.ReturnSignatureB64 {
		b64 := base64.StdEncoding.EncodeToString(result.Result.Data)
		return respuesta{OK: true, Action: "sign", Data: resultadoFirma{SignatureB64: b64, Format: string(result.Result.Format)}}
	}

	outputPath := params.OutputPath
	if outputPath == "" {
		outputPath = rutaSalida(params.InputPath, formato, params.Overwrite)
	}
	// Una llamada nativa puede terminar después de la desconexión. No iniciar
	// el commit si ya se conoce la cancelación; un commit ya iniciado no se
	// revierte ni se presenta como una operación nativa interrumpible.
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "sign", Error: err.Error()}
	}
	if err := securefile.WriteFileAtomic(outputPath, result.Result.Data, 0o600); err != nil {
		return respuesta{
			OK:     false,
			Action: "sign",
			Error:  "no se pudo guardar el resultado firmado",
		}
	}

	return respuesta{OK: true, Action: "sign", Data: resultadoFirma{OutputPath: outputPath, Format: string(result.Result.Format)}}
}

func (m *Manejador) handleFirmaMultiCofirma(ctx context.Context, raw json.RawMessage) respuesta {
	if m.MultiCofirmar == nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: m.t("error.ipc_no_configurado")}
	}
	var params paramsFirma
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: m.t("error.formato_invalido")}
	}
	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	contenido, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}
	if params.OutputPath != "" {
		if err := validarRutaEscritura(params.OutputPath); err != nil {
			return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
		}
	}

	certID, err := m.resolverCertIDPreferido(ctx, params.CertificateID, params.CertificateIndex)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	nombre := filepath.Base(params.InputPath)
	mime := mimeDesdeExtension(params.InputPath)
	formato := params.Format
	if formato == "" {
		formato = inferirFormatoRuta(params.InputPath)
	}
	signFormat, err := application.ParseSignatureFormat(formato)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	accionFirma, err := application.ParseSignatureAction(params.Action)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	doc, err := domain.NewDocument(nombre, contenido, mime)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	opciones, err := construirOpcionesFirmaIPC(params, formato)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: m.localizarErrorOpacidadLogoSello(err)}
	}
	opciones, err = m.aplicarOpcionesFirmaPredeterminadas(ctx, formato, opciones)
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	result, err := m.MultiCofirmar.Execute(ctx, application.MultiCoSignCommand{
		Document:                 doc,
		Format:                   signFormat,
		InitialAction:            accionFirma,
		PrimaryCertificateID:     certID,
		AdditionalCertificateIDs: append([]string(nil), params.AdditionalCertificateIDs...),
		Options:                  opciones,
	})
	if err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}

	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	if params.ReturnSignatureB64 {
		b64 := base64.StdEncoding.EncodeToString(result.Result.Data)
		return respuesta{OK: true, Action: "sign_multicosign", Data: resultadoFirma{SignatureB64: b64, Format: string(result.Result.Format)}}
	}

	outputPath := params.OutputPath
	if outputPath == "" {
		outputPath = rutaSalida(params.InputPath, formato, params.Overwrite)
	}
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "sign_multicosign", Error: err.Error()}
	}
	if err := securefile.WriteFileAtomic(outputPath, result.Result.Data, 0o600); err != nil {
		return respuesta{
			OK:     false,
			Action: "sign_multicosign",
			Error:  "no se pudo guardar el resultado firmado",
		}
	}
	return respuesta{OK: true, Action: "sign_multicosign", Data: resultadoFirma{OutputPath: outputPath, Format: string(result.Result.Format)}}
}

func (m *Manejador) handleFirmaLote(ctx context.Context, raw json.RawMessage) respuesta {
	var params paramsFirmaLote
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "sign_batch", Error: m.t("error.formato_invalido")}
	}
	certID, err := m.resolverCertIDPreferido(ctx, params.CertificateID, params.CertificateIndex)
	if err != nil {
		return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
	}

	inputPaths, err := recogerRutasFirmaLote(params.InputPaths, params.DirectoryPath)
	if err != nil {
		return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
	}
	if len(inputPaths) == 0 {
		return respuesta{OK: false, Action: "sign_batch", Error: "debe seleccionar al menos un fichero o una carpeta con ficheros"}
	}
	if len(inputPaths) > maxIPCBatchDocuments {
		return respuesta{
			OK:     false,
			Action: "sign_batch",
			Error: fmt.Sprintf(
				"el lote supera el máximo permitido de %d documentos",
				maxIPCBatchDocuments,
			),
		}
	}
	if params.OutputDir != "" {
		if err := validarDirectorioEscritura(params.OutputDir); err != nil {
			return respuesta{
				OK:     false,
				Action: "sign_batch",
				Error:  "la carpeta de salida seleccionada no es segura o accesible",
			}
		}
	}
	documentOverrides, err := resolverOverridesFirmaLoteIPC(params.DocumentOverrides, inputPaths)
	if err != nil {
		return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
	}
	var declaredAggregateSize int64
	for _, inputPath := range inputPaths {
		if err := validarRutaLectura(inputPath); err != nil {
			return respuesta{
				OK:     false,
				Action: "sign_batch",
				Error: fmt.Sprintf(
					"no se pudo validar el documento %s",
					filepath.Base(inputPath),
				),
			}
		}
		inputSize, err := tamanoFicheroSeguroIPC(inputPath)
		if err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
		}
		if inputSize > maxTamanoFichero {
			return respuesta{
				OK:     false,
				Action: "sign_batch",
				Error: fmt.Sprintf(
					"un documento del lote supera el tamaño máximo permitido (%d MB)",
					maxTamanoFichero/1024/1024,
				),
			}
		}
		if inputSize > maxIPCBatchTotalBytes-declaredAggregateSize {
			return respuesta{
				OK:     false,
				Action: "sign_batch",
				Error: fmt.Sprintf(
					"el lote supera el tamaño total máximo permitido (%d MB)",
					maxIPCBatchTotalBytes/1024/1024,
				),
			}
		}
		declaredAggregateSize += inputSize
	}

	inputs := make([]application.BatchItemInput, 0, len(inputPaths))
	metas := make([]metaFirmaLote, 0, len(inputPaths))
	accionFirma := params.Action
	if accionFirma == "" {
		accionFirma = "sign"
	}
	isMultiCosign := len(params.AdditionalCertificateIDs) > 0
	reservedOutputs := make(map[string]struct{}, len(inputPaths))
	var aggregateSize int64
	var preferenciasFirma ports.DocumentoConfiguracionUsuario
	preferenciasCargadas := false
	for _, inputPath := range inputPaths {
		contenido, err := leerFicheroSeguroIPC(inputPath)
		if err != nil {
			return respuesta{
				OK:     false,
				Action: "sign_batch",
				Error:  "no se pudo leer uno de los documentos seleccionados",
			}
		}
		if int64(len(contenido)) > maxIPCBatchTotalBytes-aggregateSize {
			return respuesta{
				OK:     false,
				Action: "sign_batch",
				Error: fmt.Sprintf(
					"el lote supera el tamaño total máximo permitido (%d MB)",
					maxIPCBatchTotalBytes/1024/1024,
				),
			}
		}
		aggregateSize += int64(len(contenido))

		formato := strings.TrimSpace(params.Format)
		if formato == "" {
			formato = inferirFormatoRuta(inputPath)
		}
		opciones, err := construirOpcionesFirmaLoteDocumentoIPC(params, formato, documentOverrides[normalizarClaveRutaLote(inputPath)])
		if err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: m.localizarErrorOpacidadLogoSello(err)}
		}
		if ports.NecesitaOpcionesFirmaPredeterminadas(formato, opciones) && !preferenciasCargadas {
			// Un default de UX no debe convertir el store de preferencias en
			// dependencia de disponibilidad de la firma.
			preferenciasFirma, _ = m.cargarPreferenciasFirma(ctx)
			preferenciasCargadas = true
		}
		opciones = ports.AplicarOpcionesFirmaPredeterminadas(preferenciasFirma, formato, opciones)
		outputPath, err := reservarRutaSalidaLote(
			inputPath,
			params.OutputDir,
			formato,
			params.Overwrite,
			reservedOutputs,
		)
		if err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
		}
		if !isMultiCosign {
			inputs = append(inputs, application.BatchItemInput{
				Nombre:    filepath.Base(inputPath),
				Contenido: contenido,
				TipoMIME:  mimeDesdeExtension(inputPath),
				Formato:   formato,
				Accion:    accionFirma,
				Opciones:  opciones,
			})
		}
		metas = append(metas, metaFirmaLote{
			inputPath:  inputPath,
			outputPath: outputPath,
			format:     formato,
			options:    opciones,
			content:    contenido,
		})
	}

	if isMultiCosign {
		if m.MultiCofirmar == nil {
			return respuesta{OK: false, Action: "sign_batch", Error: m.t("error.ipc_no_configurado")}
		}
		return m.handleFirmaLoteMultiCofirma(ctx, params, metas, certID)
	}
	if m.ProcesarLote == nil {
		return respuesta{OK: false, Action: "sign_batch", Error: m.t("error.ipc_no_configurado")}
	}

	cmd, err := application.NewProcessBatchCommand(inputs)
	if err != nil {
		return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
	}
	cmd.CertificateID = certID

	result, err := m.ProcesarLote.Execute(ctx, cmd)
	if err != nil {
		return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
	}

	items := make([]resultadoFirmaLoteItem, 0, len(metas))
	okCount := 0
	failCount := 0
	successIndex := 0
	for i, meta := range metas {
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
		}
		item := resultadoFirmaLoteItem{
			InputPath: meta.inputPath,
			Format:    meta.format,
		}
		if itemErr, failed := result.Errores[i]; failed {
			item.Error = mensajeErrorFirmaLoteSeguro(itemErr)
			failCount++
			items = append(items, item)
			continue
		}
		if successIndex >= len(result.Results) {
			item.Error = "resultado de lote inconsistente"
			failCount++
			items = append(items, item)
			continue
		}

		signed := result.Results[successIndex]
		successIndex++
		if err := validarRutaEscritura(meta.outputPath); err != nil {
			item.Error = "la ruta de salida ya no es segura o accesible"
			failCount++
			items = append(items, item)
			continue
		}
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
		}
		if err := securefile.WriteFileAtomic(meta.outputPath, signed.Result.Data, 0o600); err != nil {
			item.Error = "no se pudo guardar el resultado firmado"
			failCount++
			items = append(items, item)
			continue
		}

		item.OK = true
		item.OutputPath = meta.outputPath
		item.Format = string(signed.Result.Format)
		okCount++
		items = append(items, item)
	}

	resp := respuesta{
		OK:     true,
		Action: "sign_batch",
		Data: resultadoFirmaLote{
			Results:   items,
			OkCount:   okCount,
			FailCount: failCount,
		},
	}
	if failCount > 0 {
		for _, item := range items {
			if strings.TrimSpace(item.Error) == "" {
				continue
			}
			resp.Diagnostic = guidedDiagnosticResult("sign_batch", item.Error)
			break
		}
	}
	return resp
}

func (m *Manejador) handleFirmaLoteMultiCofirma(ctx context.Context, params paramsFirmaLote, metas []metaFirmaLote, certID string) respuesta {
	items := make([]resultadoFirmaLoteItem, 0, len(metas))
	okCount := 0
	failCount := 0

	for metaIndex := range metas {
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
		}
		meta := &metas[metaIndex]
		item := resultadoFirmaLoteItem{
			InputPath: meta.inputPath,
			Format:    meta.format,
		}
		signFormat, err := application.ParseSignatureFormat(meta.format)
		if err != nil {
			item.Error = mensajeErrorFirmaLoteSeguro(err)
			failCount++
			items = append(items, item)
			continue
		}
		accionFirma, err := application.ParseSignatureAction(params.Action)
		if err != nil {
			item.Error = mensajeErrorFirmaLoteSeguro(err)
			failCount++
			items = append(items, item)
			continue
		}
		doc, err := domain.NewDocument(
			filepath.Base(meta.inputPath),
			meta.content,
			mimeDesdeExtension(meta.inputPath),
		)
		meta.content = nil
		if err != nil {
			item.Error = mensajeErrorFirmaLoteSeguro(err)
			failCount++
			items = append(items, item)
			continue
		}
		result, err := m.MultiCofirmar.Execute(ctx, application.MultiCoSignCommand{
			Document:                 doc,
			Format:                   signFormat,
			InitialAction:            accionFirma,
			PrimaryCertificateID:     certID,
			AdditionalCertificateIDs: append([]string(nil), params.AdditionalCertificateIDs...),
			Options:                  meta.options,
		})
		if err != nil {
			item.Error = mensajeErrorFirmaLoteSeguro(err)
			failCount++
			items = append(items, item)
			continue
		}
		if err := validarRutaEscritura(meta.outputPath); err != nil {
			item.Error = "la ruta de salida ya no es segura o accesible"
			failCount++
			items = append(items, item)
			continue
		}
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "sign_batch", Error: err.Error()}
		}
		if err := securefile.WriteFileAtomic(meta.outputPath, result.Result.Data, 0o600); err != nil {
			item.Error = "no se pudo guardar el resultado firmado"
			failCount++
			items = append(items, item)
			continue
		}
		item.OK = true
		item.OutputPath = meta.outputPath
		item.Format = string(result.Result.Format)
		okCount++
		items = append(items, item)
	}

	return respuesta{
		OK:     true,
		Action: "sign_batch",
		Data: resultadoFirmaLote{
			Results:   items,
			OkCount:   okCount,
			FailCount: failCount,
		},
	}
}

func mensajeErrorFirmaLoteSeguro(err error) string {
	if err == nil {
		return "la firma del documento no pudo completarse"
	}
	diagnostic := application.BuildGuidedDiagnostic(
		"sign_batch",
		err.Error(),
	)
	message := strings.TrimSpace(diagnostic.UserMessage)
	if message == "" {
		message = "la firma del documento no pudo completarse"
	}
	if code := strings.TrimSpace(diagnostic.FailureCode); code != "" {
		return code + ": " + message
	}
	return message
}

// ---------------------------------------------------------------------------
// Accion de verificacion
// ---------------------------------------------------------------------------

func (m *Manejador) handleVerificacion(ctx context.Context, raw json.RawMessage) respuesta {
	if m.Verificar == nil {
		return respuesta{OK: false, Action: "verify", Error: m.t("error.verificador_no_configurado")}
	}
	var params paramsVerify
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "verify", Error: m.t("error.formato_invalido")}
	}

	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "verify", Error: err.Error()}
	}
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "verify", Error: err.Error()}
	}

	contenido, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: "verify", Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}

	doc, err := domain.NewDocument(filepath.Base(params.InputPath), contenido, mimeDesdeExtension(params.InputPath))
	if err != nil {
		return respuesta{OK: false, Action: "verify", Error: err.Error()}
	}

	cmd := application.VerifyCommand{SignedDocument: doc}
	if originalPath := strings.TrimSpace(params.OriginalPath); originalPath != "" {
		if err := validarRutaLectura(originalPath); err != nil {
			return respuesta{OK: false, Action: "verify", Error: err.Error()}
		}
		if err := verificarTamano(originalPath); err != nil {
			return respuesta{OK: false, Action: "verify", Error: err.Error()}
		}
		originalContent, err := leerFicheroSeguroIPC(originalPath)
		if err != nil {
			return respuesta{OK: false, Action: "verify", Error: m.t("error.archivo_no_encontrado", originalPath)}
		}
		originalDoc, err := domain.NewDocument(filepath.Base(originalPath), originalContent, mimeDesdeExtension(originalPath))
		if err != nil {
			return respuesta{OK: false, Action: "verify", Error: err.Error()}
		}
		cmd.OriginalDocument = &originalDoc
	}

	result, err := m.Verificar.Execute(ctx, cmd)
	if err != nil {
		return respuesta{OK: false, Action: "verify", Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "verify", Error: err.Error()}
	}

	signers := make([]string, 0, len(result.Firmantes))
	for _, f := range result.Firmantes {
		signers = append(signers, f.Subject)
	}

	return respuesta{OK: true, Action: "verify", Data: resultadoVerificacion{
		Valid:    result.Verification.Valid,
		Reason:   result.Verification.Reason,
		Details:  result.Verification.Details,
		Signers:  signers,
		Format:   result.Verification.Format,
		Coverage: result.Verification.Coverage,
		Integrity: resultadoVerificacionAspecto{
			Status:  string(result.Verification.Integrity.Status),
			Reason:  result.Verification.Integrity.Reason,
			Details: append([]string(nil), result.Verification.Integrity.Details...),
		},
		Certificate: resultadoVerificacionAspecto{
			Status:  string(result.Verification.Certificate.Status),
			Reason:  result.Verification.Certificate.Reason,
			Details: append([]string(nil), result.Verification.Certificate.Details...),
		},
		Trust: resultadoVerificacionAspecto{
			Status:  string(result.Verification.Trust.Status),
			Reason:  result.Verification.Trust.Reason,
			Details: append([]string(nil), result.Verification.Trust.Details...),
		},
		SignerSummaries: verificacionFirmantesIPC(result.Verification.SignerSummaries),
		Warnings:        append([]string(nil), result.Verification.Warnings...),
		Errors:          append([]string(nil), result.Verification.Errors...),
		Evidence:        verificacionEvidenciasIPC(result.Verification.Evidence),
	}}
}

// ---------------------------------------------------------------------------
// Acciones de protección
// ---------------------------------------------------------------------------

func (m *Manejador) handleProtectionRecipients(ctx context.Context) respuesta {
	if m.Destinatarios == nil {
		return respuesta{OK: false, Action: "protection_recipients", Error: "catalogo de destinatarios de proteccion no configurado"}
	}
	recipients, err := m.Destinatarios.List(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "protection_recipients", Error: err.Error()}
	}
	out := make([]destinatarioProteccionJSON, 0, len(recipients))
	for _, recipient := range recipients {
		profile, algorithm := describirDestinatarioProteccionIPC(recipient)
		out = append(out, destinatarioProteccionJSON{
			ID:                          recipient.ID,
			Label:                       recipient.Label,
			Origin:                      originProteccion(recipient.Origin),
			Profile:                     profile,
			Algorithm:                   algorithm,
			AuthEnvelopedDataCompatible: authEnvelopedDataCompatibleIPC(recipient),
		})
	}
	return respuesta{OK: true, Action: "protection_recipients", Data: resultadoDestinatariosProteccion{Recipients: out}}
}

func originProteccion(origin string) string {
	if origin == "importado" || origin == "otras_personas" {
		return origin
	}
	return "propio"
}

func (m *Manejador) handleProtectionRecipientImport(ctx context.Context, raw json.RawMessage) respuesta {
	const action = "protection_recipient_import"
	store, ok := m.Destinatarios.(publicProtectionRecipients)
	if !ok {
		return respuesta{OK: false, Action: action, Error: "libro de destinatarios públicos no configurado"}
	}
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: action, Error: "parámetros no válidos"}
	}
	if err := validarRutaLectura(params.Path); err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	data, err := securefile.ReadFileLimit(params.Path, 64*1024)
	if err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	recipient, err := store.ImportPublic(ctx, data)
	if err != nil {
		message := mensajeRechazoDestinatarioPublico(err)
		return respuesta{OK: false, Action: action, Error: message, Diagnostic: &resultadoDiagnosticoGuiado{
			Category: "app_local", UserMessage: message, ExpertMessage: message,
			SuggestedAction: "Seleccione un certificado X.509 público RSA vigente con permiso de cifrado.", UserCanResolveDirectly: true,
		}}
	}
	return respuesta{OK: true, Action: action, Data: map[string]string{"id": recipient.ID}}
}

func mensajeRechazoDestinatarioPublico(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "clave privada"), strings.Contains(message, "P12/PFX"):
		return "El fichero contiene una clave privada o es un P12/PFX. Seleccione solo el certificado público .cer, .crt, .pem o .der."
	case strings.Contains(message, "caducado"):
		return "El certificado está caducado o todavía no es válido. Solicite un certificado público vigente al destinatario."
	case strings.Contains(message, "KeyUsage"):
		return "El certificado no autoriza el cifrado RSA-OAEP en KeyUsage. Solicite uno con keyEncipherment."
	case strings.Contains(message, "clave pública RSA"):
		return "El perfil compatible requiere un certificado público RSA de al menos 2048 bits apto para cifrado."
	case strings.Contains(message, "64 KiB"):
		return "El certificado supera el límite de 64 KiB o está vacío."
	case strings.Contains(message, "único certificado"):
		return "Seleccione un único certificado X.509 público en el fichero PEM."
	case strings.Contains(message, "autoridad"):
		return "El certificado pertenece a una autoridad, no a un destinatario."
	case strings.Contains(message, "128 destinatarios"):
		return "El libro ya contiene el máximo de 128 destinatarios importados."
	default:
		return "No se pudo importar el certificado público. Compruebe que sea un X.509 DER o PEM apto para cifrado."
	}
}

func (m *Manejador) handleProtectionRecipientRemove(ctx context.Context, raw json.RawMessage) respuesta {
	const action = "protection_recipient_remove"
	store, ok := m.Destinatarios.(publicProtectionRecipients)
	if !ok {
		return respuesta{OK: false, Action: action, Error: "libro de destinatarios públicos no configurado"}
	}
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: action, Error: "parámetros no válidos"}
	}
	if err := store.RemovePublic(ctx, params.ID); err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	return respuesta{OK: true, Action: action}
}

func (m *Manejador) handleProtect(ctx context.Context, raw json.RawMessage) respuesta {
	defer secmem.Zeroize(raw)
	if m.Proteger == nil {
		return respuesta{OK: false, Action: "protect", Error: "caso de uso de proteccion no configurado"}
	}
	var params paramsProtection
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "protect", Error: m.t("error.formato_invalido")}
	}
	symmetricKey, err := protectionSymmetricKeyIPC(
		params.SecretB64,
		params.Options,
	)
	if err != nil {
		return respuesta{OK: false, Action: "protect", Error: err.Error()}
	}
	defer secmem.Zeroize(symmetricKey)
	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "protect", Error: err.Error()}
	}
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "protect", Error: err.Error()}
	}
	data, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: "protect", Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}
	if strings.TrimSpace(params.OutputPath) != "" {
		if err := validarRutaEscritura(params.OutputPath); err != nil {
			return respuesta{OK: false, Action: "protect", Error: err.Error()}
		}
	}
	cmd, err := application.NewProtectCommand(
		filepath.Base(params.InputPath),
		data,
		mimeDesdeExtension(params.InputPath),
		params.Profile,
		append([]string(nil), params.RecipientIDs...),
		normalizarOpcionesProteccionIPC(params.Options),
	)
	if err != nil {
		return respuesta{OK: false, Action: "protect", Error: err.Error()}
	}
	cmd.SymmetricKey = symmetricKey
	result, err := m.Proteger.Execute(ctx, cmd)
	if err != nil {
		return respuesta{OK: false, Action: "protect", Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "protect", Error: err.Error()}
	}
	outputPath := strings.TrimSpace(params.OutputPath)
	saveToDisk := params.SaveToDisk
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaProtegidaRutaIPC(params.InputPath, result.Protected.Document.Name)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if strings.EqualFold(strings.TrimSpace(params.Overwrite), "force") || strings.EqualFold(strings.TrimSpace(params.Overwrite), "true") {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "protect", Error: err.Error()}
		}
		outputPath, err = writer.Escribir(outputPath, result.Protected.Document.Content)
		if err != nil {
			return respuesta{OK: false, Action: "protect", Error: err.Error()}
		}
	}
	resp := resultadoProteccion{
		OutputPath:     outputPath,
		Profile:        string(result.Protected.Profile),
		RecipientCount: result.Protected.RecipientCount,
		DocumentName:   result.Protected.Document.Name,
		MIMEType:       result.Protected.Document.MIMEType,
	}
	if params.ReturnProtectedB64 || !saveToDisk {
		resp.ProtectedContentB64 = base64.StdEncoding.EncodeToString(result.Protected.Document.Content)
	}
	return respuesta{OK: true, Action: "protect", Data: resp}
}

func (m *Manejador) handleProtectSign(ctx context.Context, raw json.RawMessage) respuesta {
	defer secmem.Zeroize(raw)
	if m.ProtegerFirmando == nil {
		return respuesta{OK: false, Action: "protect_sign", Error: "caso de uso de proteccion firmada no configurado"}
	}
	var params paramsProtection
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: m.t("error.formato_invalido")}
	}
	symmetricKey, err := protectionSymmetricKeyIPC(
		params.SecretB64,
		params.Options,
	)
	if err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	defer secmem.Zeroize(symmetricKey)
	if len(symmetricKey) > 0 {
		return respuesta{OK: false, Action: "protect_sign", Error: "EncryptedData no admite proteger y firmar en una sola operacion"}
	}
	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	data, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}
	if strings.TrimSpace(params.OutputPath) != "" {
		if err := validarRutaEscritura(params.OutputPath); err != nil {
			return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
		}
	}
	certID, err := m.resolverCertIDPreferido(ctx, params.CertificateID, params.CertificateIndex)
	if err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	if strings.TrimSpace(certID) == "" {
		return respuesta{OK: false, Action: "protect_sign", Error: "debe seleccionarse un certificado para proteger firmando"}
	}
	options := normalizarOpcionesProteccionIPC(params.Options)
	if strings.TrimSpace(options["container"]) == "" {
		options["container"] = "signedandenvelopeddata"
	}
	cmd, err := application.NewProtectAndSignCommand(
		filepath.Base(params.InputPath),
		data,
		mimeDesdeExtension(params.InputPath),
		params.Profile,
		append([]string(nil), params.RecipientIDs...),
		certID,
		options,
	)
	if err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	result, err := m.ProtegerFirmando.Execute(ctx, cmd)
	if err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
	}
	outputPath := strings.TrimSpace(params.OutputPath)
	saveToDisk := params.SaveToDisk
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaProtegidaRutaIPC(params.InputPath, result.Protected.Document.Name)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if strings.EqualFold(strings.TrimSpace(params.Overwrite), "force") || strings.EqualFold(strings.TrimSpace(params.Overwrite), "true") {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
		}
		outputPath, err = writer.Escribir(outputPath, result.Protected.Document.Content)
		if err != nil {
			return respuesta{OK: false, Action: "protect_sign", Error: err.Error()}
		}
	}
	resp := resultadoProteccion{
		OutputPath:     outputPath,
		Profile:        string(result.Protected.Profile),
		RecipientCount: result.Protected.RecipientCount,
		DocumentName:   result.Protected.Document.Name,
		MIMEType:       result.Protected.Document.MIMEType,
		CertificateID:  result.CertificateUsed.ID,
	}
	if params.ReturnProtectedB64 || !saveToDisk {
		resp.ProtectedContentB64 = base64.StdEncoding.EncodeToString(result.Protected.Document.Content)
	}
	return respuesta{OK: true, Action: "protect_sign", Data: resp}
}

func (m *Manejador) handleUnprotect(ctx context.Context, raw json.RawMessage) respuesta {
	defer secmem.Zeroize(raw)
	if m.Desproteger == nil {
		return respuesta{OK: false, Action: "unprotect", Error: "caso de uso de desproteccion no configurado"}
	}
	var params paramsProtection
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: m.t("error.formato_invalido")}
	}
	symmetricKey, err := protectionSymmetricKeyIPC(
		params.SecretB64,
		params.Options,
	)
	if err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
	}
	defer secmem.Zeroize(symmetricKey)
	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
	}
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
	}
	data, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}
	if strings.TrimSpace(params.OutputPath) != "" {
		if err := validarRutaEscritura(params.OutputPath); err != nil {
			return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
		}
	}
	doc, err := domain.NewDocument(filepath.Base(params.InputPath), data, normalizarMIMEProtegidoIPC(params.InputPath))
	if err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
	}
	result, err := m.Desproteger.Execute(ctx, application.UnprotectCommand{
		ProtectedDocument: doc,
		Options:           normalizarOpcionesProteccionIPC(params.Options),
		SymmetricKey:      symmetricKey,
	})
	if err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
	}
	outputPath := strings.TrimSpace(params.OutputPath)
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
	}
	saveToDisk := params.SaveToDisk
	if saveToDisk && outputPath == "" {
		outputPath = construirSalidaDesprotegidaRutaIPC(params.InputPath, result.Unprotected.Document.Name)
	}
	if saveToDisk {
		politica := filesystem.PoliticaRenombrar
		if strings.EqualFold(strings.TrimSpace(params.Overwrite), "force") || strings.EqualFold(strings.TrimSpace(params.Overwrite), "true") {
			politica = filesystem.PoliticaForzar
		}
		writer := filesystem.NuevoEscritorResultado(politica)
		if err := ctx.Err(); err != nil {
			return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
		}
		outputPath, err = writer.Escribir(outputPath, result.Unprotected.Document.Content)
		if err != nil {
			return respuesta{OK: false, Action: "unprotect", Error: err.Error()}
		}
	}
	resp := resultadoDesproteccion{
		OutputPath:   outputPath,
		Profile:      string(result.Unprotected.Profile),
		RecipientID:  result.Unprotected.RecipientID,
		DocumentName: result.Unprotected.Document.Name,
		MIMEType:     result.Unprotected.Document.MIMEType,
	}
	if params.ReturnUnprotectedB64 || !saveToDisk {
		resp.UnprotectedContentB64 = base64.StdEncoding.EncodeToString(result.Unprotected.Document.Content)
	}
	return respuesta{OK: true, Action: "unprotect", Data: resp}
}

func verificacionFirmantesIPC(items []domain.VerificationSignerSummary) []resultadoVerificacionFirmante {
	if len(items) == 0 {
		return nil
	}
	out := make([]resultadoVerificacionFirmante, 0, len(items))
	for _, item := range items {
		out = append(out, resultadoVerificacionFirmante{
			ID:          item.ID,
			Subject:     item.Subject,
			Issuer:      item.Issuer,
			Fingerprint: item.Fingerprint,
		})
	}
	return out
}

func verificacionEvidenciasIPC(items []domain.VerificationEvidence) []resultadoVerificacionEvidencia {
	if len(items) == 0 {
		return nil
	}
	out := make([]resultadoVerificacionEvidencia, 0, len(items))
	for _, item := range items {
		out = append(out, resultadoVerificacionEvidencia{
			Type:    item.Type,
			Summary: item.Summary,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Acciones de hash
// ---------------------------------------------------------------------------

func (m *Manejador) handleHashCreate(ctx context.Context, raw json.RawMessage) respuesta {
	var params paramsHashCreate
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: m.t("error.formato_invalido")}
	}
	inputPath := strings.TrimSpace(params.InputPath)
	if inputPath == "" {
		return respuesta{OK: false, Action: "hash_create", Error: "la ruta de entrada no puede estar vacia"}
	}
	if esDirectorioRuta(inputPath) {
		return m.handleHashCreateDirectorio(ctx, params, inputPath)
	}
	return m.handleHashCreateFichero(ctx, params, inputPath)
}

func (m *Manejador) handleHashCreateFichero(ctx context.Context, params paramsHashCreate, inputPath string) respuesta {
	if m.CrearHash == nil {
		return respuesta{OK: false, Action: "hash_create", Error: "caso de uso de hash no configurado"}
	}
	if err := validarRutaLectura(inputPath); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	if err := verificarTamano(inputPath); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	data, err := leerFicheroSeguroIPC(inputPath)
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: m.t("error.archivo_no_encontrado", inputPath)}
	}
	format, err := application.ParseHashOutputFormat(params.Format)
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	result, err := m.CrearHash.Execute(ctx, application.CreateHashCommand{
		Data:      data,
		Algorithm: params.Algorithm,
		Format:    format,
	})
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}

	outputPath := strings.TrimSpace(params.OutputPath)
	if outputPath == "" {
		outputPath = construirRutaHashIPC(inputPath, result.Format)
	}
	if err := validarRutaEscritura(outputPath); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	writer := filesystem.NuevoEscritorResultado(filesystem.PoliticaForzar)
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	outputPath, err = writer.Escribir(outputPath, serializarHashCreadoIPC(result))
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}

	return respuesta{OK: true, Action: "hash_create", Data: resultadoHash{
		Algorithm:  result.Algorithm,
		Format:     string(result.Format),
		Hash:       result.Encoded,
		OutputPath: outputPath,
	}}
}

func (m *Manejador) handleHashCreateDirectorio(ctx context.Context, params paramsHashCreate, inputPath string) respuesta {
	if m.CrearHashDir == nil {
		return respuesta{OK: false, Action: "hash_create", Error: "caso de uso de hash de directorio no configurado"}
	}
	if err := validarDirectorioLectura(inputPath); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	format, err := parseDirectoryHashRequestFormatIPC(params.Format)
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	result, err := m.CrearHashDir.Execute(ctx, application.CreateDirectoryHashManifestCommand{
		RootPath:  inputPath,
		Algorithm: params.Algorithm,
		Format:    format,
		Recursive: params.Recursive,
	})
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}

	outputPath := strings.TrimSpace(params.OutputPath)
	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(inputPath), filepath.Base(defaultDirectoryHashFilenameIPC(filepath.Base(inputPath), result.Format)))
	}
	if err := validarRutaEscritura(outputPath); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	writer := filesystem.NuevoEscritorResultado(filesystem.PoliticaForzar)
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}
	outputPath, err = writer.Escribir(outputPath, result.Data)
	if err != nil {
		return respuesta{OK: false, Action: "hash_create", Error: err.Error()}
	}

	return respuesta{OK: true, Action: "hash_create", Data: resultadoHashDirectorio{
		Algorithm:   result.Algorithm,
		Format:      string(result.Format),
		Entries:     len(result.Manifest.Entries),
		Recursive:   result.Manifest.Recursive,
		OutputPath:  outputPath,
		ManifestB64: base64.StdEncoding.EncodeToString(result.Data),
	}}
}

func (m *Manejador) handleHashCheck(ctx context.Context, raw json.RawMessage) respuesta {
	var params paramsHashCheck
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: m.t("error.formato_invalido")}
	}
	inputPath := strings.TrimSpace(params.InputPath)
	if inputPath == "" {
		return respuesta{OK: false, Action: "hash_check", Error: "la ruta de entrada no puede estar vacia"}
	}
	if esDirectorioRuta(inputPath) {
		return m.handleHashCheckDirectorio(ctx, params, inputPath)
	}
	return m.handleHashCheckFichero(ctx, params, inputPath)
}

func (m *Manejador) handleHashCheckFichero(ctx context.Context, params paramsHashCheck, inputPath string) respuesta {
	if m.ComprobarHash == nil {
		return respuesta{OK: false, Action: "hash_check", Error: "caso de uso de comprobacion de hash no configurado"}
	}
	if err := validarRutaLectura(inputPath); err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	if err := verificarTamano(inputPath); err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	if err := validarRutaLectura(params.HashPath); err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	expectedRaw, err := leerFicheroSeguroIPC(params.HashPath)
	if err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: "no se pudo leer hashPath"}
	}
	expectedDigest, algorithm, format, err := application.ParseStoredHash(expectedRaw, params.HashPath)
	if err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	if strings.TrimSpace(params.Algorithm) != "" {
		algorithm = params.Algorithm
	}
	data, err := leerFicheroSeguroIPC(inputPath)
	if err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: m.t("error.archivo_no_encontrado", inputPath)}
	}
	result, err := m.ComprobarHash.Execute(ctx, application.CheckHashCommand{
		Data:         data,
		ExpectedHash: expectedDigest,
		Algorithm:    algorithm,
		Format:       format,
	})
	if err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "hash_check", Data: resultadoComprobarHash{
		Valid:        result.Valid,
		Algorithm:    result.Algorithm,
		Format:       string(result.Format),
		ExpectedHash: result.ExpectedEncoded,
		ActualHash:   result.ActualEncoded,
	}}
}

func (m *Manejador) handleHashCheckDirectorio(ctx context.Context, params paramsHashCheck, inputPath string) respuesta {
	if m.ComprobarHashDir == nil {
		return respuesta{OK: false, Action: "hash_check", Error: "caso de uso de comprobacion de hash de directorio no configurado"}
	}
	if err := validarDirectorioLectura(inputPath); err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	if err := validarRutaLectura(params.HashPath); err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}
	manifestData, err := leerFicheroSeguroIPC(params.HashPath)
	if err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: "no se pudo leer hashPath"}
	}
	result, err := m.ComprobarHashDir.Execute(ctx, application.CheckDirectoryHashManifestCommand{
		RootPath:     inputPath,
		ManifestData: manifestData,
		ManifestHint: params.HashPath,
	})
	if err != nil {
		return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
	}

	data := resultadoComprobarHashDirectorio{
		Valid:           result.Valid,
		Algorithm:       result.Report.Algorithm,
		Recursive:       result.Report.Recursive,
		MatchingHash:    result.Report.MatchingHash,
		NotMatchingHash: result.Report.NotMatchingHash,
		HashWithoutFile: result.Report.HashWithoutFile,
		FileWithoutHash: result.Report.FileWithoutHash,
	}
	if m.InformeHashDir != nil {
		reportData, err := m.InformeHashDir.EncodeReport(ctx, result.Report)
		if err != nil {
			return respuesta{OK: false, Action: "hash_check", Error: "no se pudo generar el informe de comprobacion"}
		}
		data.ReportB64 = base64.StdEncoding.EncodeToString(reportData)
		if params.SaveReportToDisk || strings.TrimSpace(params.OutputPath) != "" {
			outputPath := strings.TrimSpace(params.OutputPath)
			if outputPath == "" {
				outputPath = filepath.Join(filepath.Dir(inputPath), filepath.Base(defaultDirectoryHashReportFilenameIPC(filepath.Base(inputPath))))
			}
			if err := validarRutaEscritura(outputPath); err != nil {
				return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
			}
			writer := filesystem.NuevoEscritorResultado(filesystem.PoliticaForzar)
			if err := ctx.Err(); err != nil {
				return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
			}
			outputPath, err = writer.Escribir(outputPath, reportData)
			if err != nil {
				return respuesta{OK: false, Action: "hash_check", Error: err.Error()}
			}
			data.ReportOutputPath = outputPath
		}
	}

	return respuesta{OK: true, Action: "hash_check", Data: data}
}

// ---------------------------------------------------------------------------
// Accion de PDF preview
// ---------------------------------------------------------------------------

func (m *Manejador) handlePdfPreview(ctx context.Context, raw json.RawMessage) respuesta {
	if m.Preview == nil {
		return respuesta{OK: false, Action: "pdf_preview", Error: m.t("error.pdf_preview_no_configurado")}
	}
	var params paramsPdfPreview
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "pdf_preview", Error: m.t("error.formato_invalido")}
	}

	// Validar ruta: solo se permiten ficheros PDF en ubicaciones de usuario.
	if err := validarRutaLectura(params.Path); err != nil {
		return respuesta{OK: false, Action: "pdf_preview", Error: err.Error()}
	}
	if strings.ToLower(filepath.Ext(params.Path)) != ".pdf" {
		return respuesta{OK: false, Action: "pdf_preview", Error: "solo se admiten ficheros PDF"}
	}
	if err := verificarTamano(params.Path); err != nil {
		return respuesta{OK: false, Action: "pdf_preview", Error: err.Error()}
	}

	result, err := m.Preview.Ejecutar(ctx, application.PdfPreviewCommand{
		Ruta:   params.Path,
		Pagina: params.Page,
	})
	if err != nil {
		return respuesta{OK: false, Action: "pdf_preview", Error: err.Error()}
	}

	return respuesta{OK: true, Action: "pdf_preview", Data: resultadoPdfPreview{
		Data:        result.DataB64,
		Width:       result.Ancho,
		Height:      result.Alto,
		CurrentPage: result.PaginaActual,
		TotalPages:  result.TotalPaginas,
	}}
}

// ---------------------------------------------------------------------------
// Acciones de servicio
// ---------------------------------------------------------------------------

func (m *Manejador) handleServiceStatus(ctx context.Context) respuesta {
	if m.Servicio == nil {
		return respuesta{OK: false, Action: "service_status", Error: m.t("error.servicio_no_configurado")}
	}
	estado, err := m.Servicio.Estado(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "service_status", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "service_status", Data: resultadoEstadoServicio{
		Installed: estado.Instalado,
		Running:   estado.Activo,
		Platform:  estado.Plataforma,
		Method:    estado.Metodo,
	}}
}

func (m *Manejador) handleServiceInstall(ctx context.Context, raw json.RawMessage) respuesta {
	if m.Servicio == nil {
		return respuesta{OK: false, Action: "service_install", Error: m.t("error.servicio_no_configurado")}
	}
	var params paramsInstallService
	_ = json.Unmarshal(raw, &params)
	if err := m.Servicio.Instalar(ctx, params.IpcSocket); err != nil {
		return respuesta{OK: false, Action: "service_install", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "service_install", Data: m.t("info.servicio_instalado")}
}

func (m *Manejador) handleServiceUninstall(ctx context.Context) respuesta {
	if m.Servicio == nil {
		return respuesta{OK: false, Action: "service_uninstall", Error: m.t("error.servicio_no_configurado")}
	}
	if err := m.Servicio.Desinstalar(ctx); err != nil {
		return respuesta{OK: false, Action: "service_uninstall", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "service_uninstall", Data: m.t("info.servicio_desinstalado")}
}

func (m *Manejador) handleServiceStart(ctx context.Context) respuesta {
	if m.Servicio == nil {
		return respuesta{OK: false, Action: "service_start", Error: m.t("error.servicio_no_configurado")}
	}
	if err := m.Servicio.Iniciar(ctx); err != nil {
		return respuesta{OK: false, Action: "service_start", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "service_start", Data: m.t("info.servicio_iniciado")}
}

func (m *Manejador) handleServiceStop(ctx context.Context) respuesta {
	if m.Servicio == nil {
		return respuesta{OK: false, Action: "service_stop", Error: m.t("error.servicio_no_configurado")}
	}
	if err := m.Servicio.Detener(ctx); err != nil {
		return respuesta{OK: false, Action: "service_stop", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "service_stop", Data: m.t("info.servicio_detenido")}
}

// ---------------------------------------------------------------------------
// Acciones de configuracion
// ---------------------------------------------------------------------------

func (m *Manejador) handleGetSettings(ctx context.Context) respuesta {
	if m.Settings == nil {
		return respuesta{OK: true, Action: "get_settings", Data: map[string]any{}}
	}
	if tipado, ok := m.Settings.(ports.ConfiguracionUsuarioTipada); ok {
		doc, err := tipado.CargarDocumento(ctx)
		if err != nil {
			return respuesta{OK: false, Action: "get_settings", Error: err.Error()}
		}
		return respuesta{OK: true, Action: "get_settings", Data: doc.Mapa()}
	}
	datos, err := m.Settings.Cargar(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "get_settings", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "get_settings", Data: datos}
}

func (m *Manejador) handleSaveSettings(ctx context.Context, raw json.RawMessage) respuesta {
	if m.Settings == nil {
		return respuesta{OK: false, Action: "save_settings", Error: m.t("error.settings_no_configurado")}
	}
	var datos map[string]any
	if err := json.Unmarshal(raw, &datos); err != nil {
		return respuesta{OK: false, Action: "save_settings", Error: m.t("error.formato_invalido")}
	}
	if err := ports.ValidarSinCredencialesProxyEnClaro(datos); err != nil {
		return respuesta{OK: false, Action: "save_settings", Error: err.Error()}
	}
	if err := ports.ValidarDuracionCompatibilidadWeb(datos); err != nil {
		return respuesta{OK: false, Action: "save_settings", Error: err.Error()}
	}
	if err := ports.ValidarOpacidadLogoSello(datos); err != nil {
		return respuesta{OK: false, Action: "save_settings", Error: m.localizarErrorOpacidadLogoSello(err)}
	}
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	// La referencia al secreto solo puede cambiar mediante las acciones
	// proxy_secret_store/proxy_secret_delete. Una GUI que guarde el resto de
	// preferencias no debe poder borrarla ni sustituirla con un ID arbitrario.
	actual, err := m.loadSettingsDocument(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "save_settings", Error: m.t("error.settings_no_configurado")}
	}
	delete(datos, "proxySecretId")
	delete(datos, "proxyRealm")
	siguiente := ports.DocumentoConfiguracionUsuarioDesdeMapa(datos)
	siguiente.Proxy.SecretID = actual.Proxy.SecretID
	siguiente.Proxy.Realm = actual.Proxy.Realm
	if err := m.saveSettingsDocument(ctx, siguiente); err != nil {
		return respuesta{OK: false, Action: "save_settings", Error: m.t("error.settings_no_configurado")}
	}
	return respuesta{OK: true, Action: "save_settings", Data: m.t("info.settings_guardados")}
}

const (
	maxProxyRealmBytes    = 256
	maxProxyUsernameBytes = 256
	maxProxyPasswordBytes = 4096
	maxProxySecretIDBytes = 128
)

func (m *Manejador) handleProxySecretStore(ctx context.Context, raw json.RawMessage) respuesta {
	defer secmem.Zeroize(raw)
	if m.ProxySecrets == nil || m.Settings == nil {
		return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("error.settings_no_configurado")}
	}

	var params paramsProxySecretStore
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("error.formato_invalido")}
	}
	defer secmem.Zeroize(params.Password)

	realm := strings.TrimSpace(params.Realm)
	username := strings.TrimSpace(params.Username)
	if !validProxyCredentialText(realm, maxProxyRealmBytes) ||
		!validProxyCredentialText(username, maxProxyUsernameBytes) ||
		!validProxyPassword(params.Password) {
		return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("error.formato_invalido")}
	}

	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	previous, err := m.loadSettingsDocument(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("error.settings_no_configurado")}
	}
	oldID := proxySecretID(previous.Proxy.SecretID)

	descriptor, err := m.ProxySecrets.Store(ctx, realm, ports.ProxySecretMaterial{
		Realm:    realm,
		Username: username,
		Password: params.Password,
	})
	if err != nil || !validProxySecretID(descriptor.ID) {
		return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("Almacén seguro del proxy no disponible")}
	}
	newID := strings.TrimSpace(descriptor.ID)

	next := previous
	next.Proxy.SecretID = stringPtrIPC(newID)
	next.Proxy.Realm = stringPtrIPC(realm)
	if err := m.saveSettingsDocument(ctx, next); err != nil {
		// El nuevo secreto todavía no es alcanzable desde settings. Se elimina
		// como rollback; el secreto anterior sigue siendo la referencia activa.
		_ = m.ProxySecrets.Delete(ctx, newID)
		return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("error.settings_no_configurado")}
	}

	rotated := oldID != "" && oldID != newID
	if rotated {
		if err := m.ProxySecrets.Delete(ctx, oldID); err != nil {
			// La rotación solo se considera completa si se retira el secreto
			// anterior. Restaurar primero la referencia antigua y retirar luego
			// la nueva evita perder una credencial válida.
			if rollbackErr := m.saveSettingsDocument(ctx, previous); rollbackErr == nil {
				_ = m.ProxySecrets.Delete(ctx, newID)
			}
			return respuesta{OK: false, Action: "proxy_secret_store", Error: m.t("Almacén seguro del proxy no disponible")}
		}
	}

	return respuesta{OK: true, Action: "proxy_secret_store", Data: resultadoCredencialProxy{
		Configured: true,
		Realm:      realm,
		Username:   username,
		Rotated:    rotated,
	}}
}

func (m *Manejador) handleProxySecretDelete(ctx context.Context) respuesta {
	if m.ProxySecrets == nil || m.Settings == nil {
		return respuesta{OK: false, Action: "proxy_secret_delete", Error: m.t("error.settings_no_configurado")}
	}

	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	previous, err := m.loadSettingsDocument(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "proxy_secret_delete", Error: m.t("error.settings_no_configurado")}
	}
	oldID := proxySecretID(previous.Proxy.SecretID)
	if oldID == "" {
		return respuesta{OK: true, Action: "proxy_secret_delete", Data: resultadoCredencialProxy{Configured: false}}
	}
	if !validProxySecretID(oldID) {
		return respuesta{OK: false, Action: "proxy_secret_delete", Error: m.t("error.formato_invalido")}
	}

	next := previous
	next.Proxy.SecretID = nil
	next.Proxy.Realm = nil
	if err := m.saveSettingsDocument(ctx, next); err != nil {
		// Si settings no cambia, el secreto sigue siendo necesario y no se toca.
		return respuesta{OK: false, Action: "proxy_secret_delete", Error: m.t("error.settings_no_configurado")}
	}
	if err := m.ProxySecrets.Delete(ctx, oldID); err != nil {
		// Delete ha fallado, por lo que el secreto continúa existiendo. Restaurar
		// su referencia mantiene un estado coherente y permite reintentar.
		_ = m.saveSettingsDocument(ctx, previous)
		return respuesta{OK: false, Action: "proxy_secret_delete", Error: m.t("Almacén seguro del proxy no disponible")}
	}
	return respuesta{OK: true, Action: "proxy_secret_delete", Data: resultadoCredencialProxy{Configured: false}}
}

func (m *Manejador) loadSettingsDocument(ctx context.Context) (ports.DocumentoConfiguracionUsuario, error) {
	if typed, ok := m.Settings.(ports.ConfiguracionUsuarioTipada); ok {
		return typed.CargarDocumento(ctx)
	}
	data, err := m.Settings.Cargar(ctx)
	if err != nil {
		return ports.DocumentoConfiguracionUsuario{}, err
	}
	return ports.DocumentoConfiguracionUsuarioDesdeMapa(data), nil
}

func (m *Manejador) saveSettingsDocument(ctx context.Context, doc ports.DocumentoConfiguracionUsuario) error {
	if typed, ok := m.Settings.(ports.ConfiguracionUsuarioTipada); ok {
		return typed.GuardarDocumento(ctx, doc)
	}
	return m.Settings.Guardar(ctx, doc.Mapa())
}

func (m *Manejador) aplicarOpcionesFirmaPredeterminadas(ctx context.Context, formato string, explicitas map[string]string) (map[string]string, error) {
	if !ports.NecesitaOpcionesFirmaPredeterminadas(formato, explicitas) {
		return explicitas, nil
	}
	doc, err := m.cargarPreferenciasFirma(ctx)
	if err != nil {
		// PAdES conserva el subfiltro ETSI seguro del motor si el documento de
		// preferencias no puede cargarse.
		return explicitas, nil
	}
	return ports.AplicarOpcionesFirmaPredeterminadas(doc, formato, explicitas), nil
}

func (m *Manejador) cargarPreferenciasFirma(ctx context.Context) (ports.DocumentoConfiguracionUsuario, error) {
	if m.Settings == nil {
		return ports.DocumentoConfiguracionUsuario{}, nil
	}
	doc, err := m.loadSettingsDocument(ctx)
	if err != nil {
		return ports.DocumentoConfiguracionUsuario{}, errors.New("no se pudieron cargar los ajustes de firma")
	}
	return doc, nil
}

func validProxyCredentialText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validProxyPassword(password []byte) bool {
	if len(password) == 0 || len(password) > maxProxyPasswordBytes || !utf8.Valid(password) {
		return false
	}
	for len(password) > 0 {
		r, size := utf8.DecodeRune(password)
		if unicode.IsControl(r) {
			return false
		}
		password = password[size:]
	}
	return true
}

func validProxySecretID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > maxProxySecretIDBytes || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if r > unicode.MaxASCII ||
			!(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(id, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(base) == 4 &&
			(strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
			base[3] >= '1' && base[3] <= '9') {
		return false
	}
	return true
}

func proxySecretID(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPtrIPC(value string) *string {
	out := value
	return &out
}

func (m *Manejador) handleProxySecretStoreStatus(ctx context.Context) respuesta {
	runtimeMode := m.runtimeProxyMode(ctx)
	if m.ProxySecrets == nil {
		return respuesta{OK: true, Action: "proxy_secret_store_status", Data: resultadoEstadoProxySecretStore{
			Available:   false,
			Platform:    "",
			Backend:     "none",
			Reason:      "proxy secret store no configurado",
			RuntimeMode: runtimeMode,
		}}
	}
	status, err := m.ProxySecrets.Status(ctx)
	if err != nil {
		return respuesta{OK: false, Action: "proxy_secret_store_status", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "proxy_secret_store_status", Data: resultadoEstadoProxySecretStore{
		Available:   status.Available,
		Platform:    status.Platform,
		Backend:     status.Backend,
		Reason:      status.Reason,
		RuntimeMode: runtimeMode,
	}}
}

func (m *Manejador) runtimeProxyMode(ctx context.Context) string {
	doc, err := usersettings.CargarDocumentoCompat(ctx, m.ConfigDir)
	if err != nil {
		return "fail-closed"
	}
	cfg, err := proxysecretstore.ResolveRuntimeProxy(ctx, doc.Proxy, m.ProxySecrets)
	if err == nil {
		return classifyRuntimeProxyModeIPC(cfg)
	}
	return "fail-closed"
}

func classifyRuntimeProxyModeIPC(cfg proxysecretstore.RuntimeProxyConfig) string {
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

func describirDestinatarioProteccionIPC(recipient domain.ProtectionRecipient) (profile, algorithm string) {
	switch {
	case len(recipient.MLKEM768PublicKey) > 0 && len(recipient.X25519PublicKey) > 0:
		return "alto", "mlkem768+x25519"
	case len(recipient.RSAOAEP256PublicKeyDER) > 0:
		return "compat", "rsa-oaep"
	default:
		return "compat", ""
	}
}

func authEnvelopedDataCompatibleIPC(recipient domain.ProtectionRecipient) bool {
	if strings.TrimSpace(recipient.ID) == "" ||
		len(recipient.CertificateDER) == 0 || len(recipient.RSAOAEP256PublicKeyDER) == 0 ||
		len(recipient.CertificateDER) > 1<<20 || len(recipient.RSAOAEP256PublicKeyDER) > 64<<10 {
		return false
	}
	certificate, err := x509.ParseCertificate(recipient.CertificateDER)
	if err != nil || certificate.SerialNumber == nil || certificate.SerialNumber.Sign() <= 0 ||
		len(certificate.SerialNumber.Bytes()) > 20 || len(certificate.RawIssuer) == 0 ||
		len(certificate.RawIssuer) > 16<<10 {
		return false
	}
	certificateKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok || certificateKey.N == nil || certificateKey.N.BitLen() < 2048 ||
		certificateKey.N.BitLen() > 8192 {
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

func normalizarOpcionesProteccionIPC(options map[string]string) map[string]string {
	if len(options) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(options))
	for k, v := range options {
		out[k] = v
	}
	return out
}

func protectionSymmetricKeyIPC(
	secretB64 *[]byte,
	options map[string]string,
) ([]byte, error) {
	if secretB64 == nil {
		return nil, nil
	}
	secret := *secretB64
	if _, legacyProvided := options["secret_b64"]; legacyProvided {
		secmem.Zeroize(secret)
		return nil, errors.New("la petición contiene dos claves simétricas incompatibles")
	}
	if len(secret) != 32 {
		secmem.Zeroize(secret)
		return nil, errors.New("secretB64 debe decodificar exactamente 32 bytes para AES-256-GCM")
	}
	return secret, nil
}

func construirSalidaProtegidaRutaIPC(inputPath, documentName string) string {
	safeDocumentName := filepath.Base(strings.TrimSpace(documentName))
	lowerDocumentName := strings.ToLower(safeDocumentName)
	if strings.HasSuffix(lowerDocumentName, ".authenveloped.p7m") ||
		strings.HasSuffix(lowerDocumentName, ".encrypted.p7m") ||
		strings.HasSuffix(lowerDocumentName, ".signedenveloped.p7m") {
		return filepath.Join(filepath.Dir(inputPath), safeDocumentName)
	}
	ext := filepath.Ext(safeDocumentName)
	if strings.EqualFold(ext, ".enveloped") || strings.EqualFold(ext, ".afp") {
		return filepath.Join(filepath.Dir(inputPath), safeDocumentName)
	}
	container := ".afp"
	if strings.EqualFold(ext, ".p7m") || strings.EqualFold(ext, ".p7c") || strings.EqualFold(ext, ".pem") {
		container = ".enveloped"
	}
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if strings.TrimSpace(base) == "" {
		base = strings.TrimSuffix(documentName, filepath.Ext(documentName))
	}
	if strings.TrimSpace(base) == "" {
		base = "documento_protegido"
	}
	return filepath.Join(filepath.Dir(inputPath), base+container)
}

func construirSalidaDesprotegidaRutaIPC(inputPath, documentName string) string {
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if strings.TrimSpace(base) == "" {
		base = strings.TrimSuffix(documentName, filepath.Ext(documentName))
	}
	if strings.TrimSpace(base) == "" {
		base = "documento"
	}
	ext := filepath.Ext(documentName)
	if strings.TrimSpace(ext) == "" {
		ext = filepath.Ext(base)
	}
	return filepath.Join(filepath.Dir(inputPath), base+"_desprotegido"+ext)
}

func normalizarMIMEProtegidoIPC(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".p7m":
		return domain.MIMETypeProtectedCMS
	case ".afp", ".enveloped":
		return domain.MIMETypeProtectedEnvelope
	default:
		return domain.MIMETypeProtectedEnvelope
	}
}

// ---------------------------------------------------------------------------
// Accion de importacion de certificado
// ---------------------------------------------------------------------------

func (m *Manejador) handleImportCert(ctx context.Context, raw json.RawMessage) respuesta {
	if m.Importador == nil {
		return respuesta{OK: false, Action: "import_certificate", Error: m.t("error.importador_no_configurado")}
	}
	var params paramsImportCert
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "import_certificate", Error: m.t("error.formato_invalido")}
	}
	data, err := base64.StdEncoding.DecodeString(params.P12B64)
	if err != nil {
		return respuesta{OK: false, Action: "import_certificate", Error: m.t("error.formato_invalido")}
	}
	defer secmem.Zeroize(data)
	// Limitar tamano del certificado a 10 MB (P12 normales son < 10 KB).
	if len(data) > 10*1024*1024 {
		return respuesta{OK: false, Action: "import_certificate", Error: "fichero de certificado demasiado grande"}
	}
	if err := useIPCPassword(
		params.PasswordB64,
		params.Password,
		func(password string) error {
			_, importErr := m.Importador.Import(ctx, data, password)
			return importErr
		},
	); err != nil {
		return respuesta{OK: false, Action: "import_certificate", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "import_certificate", Data: m.t("info.certificado_importado")}
}

func (m *Manejador) handleCertificateAccessOptions(ctx context.Context) respuesta {
	if m.CertificateAccess == nil {
		return respuesta{OK: false, Action: "certificate_access_options", Error: "El acceso guiado a certificados no está configurado."}
	}
	result, err := m.CertificateAccess.List(ctx, application.ListCertificateAccessOptionsCommand{})
	if err != nil {
		return respuesta{OK: false, Action: "certificate_access_options", Error: err.Error()}
	}
	data := certificateAccessOptionsJSON{
		DetectedBrowser:  result.Options.DetectedBrowser,
		PreferredManager: result.Options.PreferredManager,
		PreferredTarget:  result.Options.PreferredTarget,
	}
	for _, manager := range result.Options.Managers {
		data.Managers = append(data.Managers, certificateAccessManagerJSON{
			ID: manager.ID, Label: manager.Label, Recommended: manager.Recommended,
		})
	}
	for _, target := range result.Options.ImportTargets {
		data.ImportTargets = append(data.ImportTargets, certificateImportTargetJSON{
			ID: target.ID, Label: target.Label, Browser: target.Browser, Recommended: target.Recommended,
		})
	}
	return respuesta{OK: true, Action: "certificate_access_options", Data: data}
}

func (m *Manejador) handleOpenCertificateManager(ctx context.Context, raw json.RawMessage) respuesta {
	if m.CertificateAccess == nil {
		return respuesta{OK: false, Action: "open_certificate_manager", Error: "El acceso guiado a certificados no está configurado."}
	}
	var params paramsOpenCertificateManager
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "open_certificate_manager", Error: "La petición para abrir el gestor no es válida."}
	}
	if err := m.CertificateAccess.OpenManager(ctx, application.OpenCertificateManagerCommand{ManagerID: params.ManagerID}); err != nil {
		return respuesta{OK: false, Action: "open_certificate_manager", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "open_certificate_manager", Data: "Gestor de certificados abierto."}
}

func (m *Manejador) handleImportCertificateToStore(ctx context.Context, raw json.RawMessage) respuesta {
	if m.CertificateAccess == nil {
		return respuesta{OK: false, Action: "import_certificate_to_store", Error: "La importación guiada no está configurada."}
	}
	var params paramsImportCertificateToStore
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "import_certificate_to_store", Error: "La petición de importación no es válida."}
	}
	data, err := decodeIPCCredential(params.CredentialB64)
	if err != nil {
		return respuesta{OK: false, Action: "import_certificate_to_store", Error: err.Error()}
	}
	defer secmem.Zeroize(data)
	if err := useIPCPassword(
		params.PasswordB64,
		params.Password,
		func(password string) error {
			return m.CertificateAccess.Import(ctx, application.ImportCertificateToStoreCommand{
				TargetID: params.TargetID,
				Data:     data,
				Password: password,
			})
		},
	); err != nil {
		return respuesta{OK: false, Action: "import_certificate_to_store", Error: err.Error()}
	}
	return respuesta{OK: true, Action: "import_certificate_to_store", Data: "Certificado importado en el almacén seleccionado."}
}

func (m *Manejador) handleUseTemporaryCertificate(ctx context.Context, raw json.RawMessage) respuesta {
	if m.TemporaryCertificates == nil {
		return respuesta{OK: false, Action: "use_temporary_certificate", Error: "El uso temporal de certificados no está configurado."}
	}
	var params paramsUseTemporaryCertificate
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "use_temporary_certificate", Error: "La petición de credencial temporal no es válida."}
	}
	data, err := decodeIPCCredential(params.CredentialB64)
	if err != nil {
		return respuesta{OK: false, Action: "use_temporary_certificate", Error: err.Error()}
	}
	defer secmem.Zeroize(data)
	var result application.UseTemporaryCertificateResult
	err = useIPCPassword(
		params.PasswordB64,
		params.Password,
		func(password string) error {
			var useErr error
			result, useErr = m.TemporaryCertificates.Use(ctx, application.UseTemporaryCertificateCommand{
				Data: data, Password: password,
			})
			return useErr
		},
	)
	if err != nil {
		return respuesta{OK: false, Action: "use_temporary_certificate", Error: err.Error()}
	}
	if m.Catalogo != nil {
		if refs, listErr := m.Catalogo.List(ctx); listErr == nil {
			m.setUltimosCerts(refs)
		}
	}
	return respuesta{OK: true, Action: "use_temporary_certificate", Data: temporaryCertificateJSON{
		ID: result.Certificate.ID, Subject: result.Certificate.Subject,
		Fingerprint: result.Certificate.Fingerprint, Temporary: true,
	}}
}

func (m *Manejador) handleRemoveTemporaryCertificate(ctx context.Context, raw json.RawMessage) respuesta {
	if m.TemporaryCertificates == nil {
		return respuesta{OK: false, Action: "remove_temporary_certificate", Error: "El uso temporal de certificados no está configurado."}
	}
	var params paramsRemoveTemporaryCertificate
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: "remove_temporary_certificate", Error: "La petición para retirar la credencial temporal no es válida."}
	}
	if err := m.TemporaryCertificates.Remove(ctx, application.RemoveTemporaryCertificateCommand{
		CertificateID: params.CertificateID,
	}); err != nil {
		return respuesta{OK: false, Action: "remove_temporary_certificate", Error: err.Error()}
	}
	if m.Catalogo != nil {
		if refs, listErr := m.Catalogo.List(ctx); listErr == nil {
			m.setUltimosCerts(refs)
		}
	}
	return respuesta{OK: true, Action: "remove_temporary_certificate", Data: "Credencial temporal retirada de la sesión."}
}

func (m *Manejador) handleClearTemporaryCertificates(ctx context.Context) respuesta {
	if m.TemporaryCertificates == nil {
		return respuesta{OK: false, Action: "clear_temporary_certificates", Error: "El uso temporal de certificados no está configurado."}
	}
	if err := m.TemporaryCertificates.Clear(ctx); err != nil {
		return respuesta{OK: false, Action: "clear_temporary_certificates", Error: err.Error()}
	}
	if m.Catalogo != nil {
		if refs, listErr := m.Catalogo.List(ctx); listErr == nil {
			m.setUltimosCerts(refs)
		} else {
			m.setUltimosCerts(nil)
		}
	} else {
		m.setUltimosCerts(nil)
	}
	return respuesta{OK: true, Action: "clear_temporary_certificates", Data: "Credenciales temporales retiradas antes del modo residente."}
}

func useIPCPassword(
	passwordB64 *[]byte,
	legacyPassword string,
	use func(string) error,
) error {
	var password []byte
	if passwordB64 != nil {
		password = *passwordB64
		if legacyPassword != "" {
			defer secmem.Zeroize(password)
			return errors.New("la petición contiene dos contraseñas incompatibles")
		}
	} else {
		// Compatibilidad temporal con Qt y clientes desktop-ipc-v1 anteriores.
		// Los clientes nuevos deben enviar passwordB64 para no materializar el
		// secreto como string durante el unmarshal.
		password = []byte(legacyPassword)
	}
	if len(password) > maxIPCPasswordBytes {
		secmem.Zeroize(password)
		return errors.New("la contraseña supera el tamaño permitido")
	}
	if !utf8.Valid(password) {
		secmem.Zeroize(password)
		return errors.New("la contraseña no contiene texto UTF-8 válido")
	}
	for _, value := range password {
		if value == 0 {
			secmem.Zeroize(password)
			return errors.New("la contraseña contiene un carácter no permitido")
		}
	}
	return secmem.UseReadOnlyString(password, use)
}

func decodeIPCCredential(credential []byte) ([]byte, error) {
	if len(credential) == 0 {
		return nil, errors.New("debe seleccionar una credencial P12/PFX o PEM")
	}
	const maxCredentialBytes = 2 * 1024 * 1024
	if len(credential) > maxCredentialBytes {
		secmem.Zeroize(credential)
		return nil, errors.New("la credencial supera el tamaño permitido por la interfaz local")
	}
	return credential, nil
}

// ---------------------------------------------------------------------------
// Acciones de diagnostico TLS
// ---------------------------------------------------------------------------

func (m *Manejador) handleTlsDiagnostics(_ context.Context) respuesta {
	tlsDir := filepath.Join(m.configDirSeguro(), "tls")
	return respuesta{
		OK:     true,
		Action: "tls_diagnostics",
		Data:   diagnosticarAlmacenTLSIPC(tlsDir),
	}
}

func (m *Manejador) handleClearTlsTrust(ctx context.Context) respuesta {
	tlsDir := filepath.Join(m.configDirSeguro(), "tls")
	artifactPaths := tlsIPCManagedArtifactPaths(tlsDir)
	if err := validateTLSIPCCleanupTargets(tlsDir, artifactPaths); err != nil {
		return respuesta{
			OK:        false,
			Action:    "clear_tls_trust",
			ErrorCode: "tls_artifact_cleanup_failed",
			Error:     "no se pudo validar el almacén TLS local",
		}
	}

	deps := m.tlsIPCDependencies()
	rootCertFile := filepath.Join(
		tlsDir,
		tlsIPCCertificatePrefix+"-root.crt.pem",
	)
	if err := deps.removeManagedTrust(ctx, rootCertFile); err != nil {
		// La prueba de propiedad y los artefactos locales se conservan para
		// permitir un reintento seguro. Nunca se borra primero la evidencia que
		// autoriza retirar la CA de CurrentUser/ROOT.
		return respuesta{
			OK:        false,
			Action:    "clear_tls_trust",
			ErrorCode: "tls_trust_remove_failed",
			Error:     "no se pudo retirar la confianza TLS local gestionada",
		}
	}

	eliminados, err := removeTLSIPCManagedArtifacts(artifactPaths)
	if err != nil {
		return respuesta{
			OK:        false,
			Action:    "clear_tls_trust",
			ErrorCode: "tls_artifact_cleanup_failed",
			Error:     "no se pudieron retirar los artefactos TLS locales",
		}
	}
	return respuesta{OK: true, Action: "clear_tls_trust", Data: eliminados}
}

func (m *Manejador) handleExportDiagnostic(ctx context.Context) respuesta {
	total, canSign := m.diagnosticCertificateCounts(ctx)
	tlsDir := filepath.Join(m.configDirSeguro(), "tls")

	return respuesta{OK: true, Action: "export_diagnostic", Data: resultadoDiagnostico{
		Certificates: total,
		CanSign:      canSign,
		TLSStore:     diagnosticarAlmacenTLSIPC(tlsDir),
	}}
}

func diagnosticarAlmacenTLSIPC(dir string) resultadoEstadoTLSIPC {
	resultado := resultadoEstadoTLSIPC{State: "not_created"}
	handle, err := securefile.OpenDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return resultado
	}
	if err != nil {
		resultado.State = "unavailable"
		return resultado
	}
	defer handle.Close()

	entries, err := handle.ReadDir(-1)
	if err != nil {
		resultado.State = "unavailable"
		return resultado
	}

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
	return resultado
}

func (m *Manejador) handleInstallPublicRoots(ctx context.Context) respuesta {
	tlsDir := filepath.Join(m.configDirSeguro(), "tls")
	deps := m.tlsIPCDependencies()
	_, _, rootCertFile, source, err :=
		deps.ensureBrowserCompatibleCertificate(
			tlsDir,
			tlsIPCCertificatePrefix,
		)
	expectedRootCertFile := filepath.Join(
		tlsDir,
		tlsIPCCertificatePrefix+"-root.crt.pem",
	)
	if err != nil ||
		source != "grxfirma-local-ca" ||
		filepath.Clean(rootCertFile) != filepath.Clean(expectedRootCertFile) {
		return respuesta{
			OK:        false,
			Action:    "install_public_roots",
			ErrorCode: "tls_certificate_prepare_failed",
			Error:     "no se pudo preparar el certificado TLS local gestionado",
		}
	}
	if err := deps.ensureManagedTrust(ctx, rootCertFile); err != nil {
		return respuesta{
			OK:        false,
			Action:    "install_public_roots",
			ErrorCode: "tls_trust_install_failed",
			Error:     "no se pudo instalar la confianza TLS local gestionada",
		}
	}
	return respuesta{OK: true, Action: "install_public_roots", Data: m.t("info.raices_instaladas")}
}

func (m *Manejador) tlsIPCDependencies() tlsIPCDependencies {
	deps := defaultTLSIPCDependencies()
	if m.tlsDeps.ensureBrowserCompatibleCertificate != nil {
		deps.ensureBrowserCompatibleCertificate =
			m.tlsDeps.ensureBrowserCompatibleCertificate
	}
	if m.tlsDeps.ensureManagedTrust != nil {
		deps.ensureManagedTrust = m.tlsDeps.ensureManagedTrust
	}
	if m.tlsDeps.removeManagedTrust != nil {
		deps.removeManagedTrust = m.tlsDeps.removeManagedTrust
	}
	return deps
}

func (m *Manejador) diagnosticCertificateCounts(
	ctx context.Context,
) (total, canSign int) {
	if m.Catalogo == nil {
		return 0, 0
	}
	certs, err := m.Catalogo.List(ctx)
	if err != nil {
		return 0, 0
	}
	total = len(certs)

	now := time.Now()
	for _, cert := range certs {
		if cert.NotAfter.IsZero() || cert.IsExpired(now) {
			continue
		}
		if cert.HasSigningKey {
			canSign++
		}
	}
	return total, canSign
}

func tlsIPCManagedArtifactPaths(tlsDir string) []string {
	rootCertFile := filepath.Join(
		tlsDir,
		tlsIPCCertificatePrefix+"-root.crt.pem",
	)
	return []string{
		filepath.Join(tlsDir, tlsIPCCertificatePrefix+".crt.pem"),
		filepath.Join(tlsDir, tlsIPCCertificatePrefix+".key.pem"),
		rootCertFile,
		filepath.Join(
			tlsDir,
			tlsIPCCertificatePrefix+"-root.key.pem",
		),
		rootCertFile + ".trustcache.json",
		rootCertFile + ".grxfirma-trust.json",
	}
}

func validateTLSIPCCleanupTargets(
	tlsDir string,
	artifactPaths []string,
) error {
	info, err := os.Lstat(tlsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("el almacén TLS local no es un directorio seguro")
	}

	handle, err := securefile.OpenDir(tlsDir)
	if err != nil {
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}

	for _, path := range artifactPaths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return errors.New("un artefacto TLS gestionado es un directorio")
		}
	}
	return nil
}

func removeTLSIPCManagedArtifacts(artifactPaths []string) (int, error) {
	removed := 0
	for _, path := range artifactPaths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		if info.IsDir() {
			return removed, errors.New(
				"un artefacto TLS gestionado es un directorio",
			)
		}
		if err := os.Remove(path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (m *Manejador) configDirSeguro() string {
	if strings.TrimSpace(m.ConfigDir) != "" {
		return m.ConfigDir
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return appdirs.Config(home)
	}
	return filepath.Join(os.TempDir(), "grxfirma")
}

// ---------------------------------------------------------------------------
// Validacion de rutas (seguridad)
// ---------------------------------------------------------------------------

// validarRutaLectura comprueba que la ruta es segura para leer:
//   - Es un fichero regular (no dispositivo, no symlink a zona sensible).
//   - No apunta a directorios de sistema prohibidos.
func validarRutaLectura(ruta string) error {
	if ruta == "" {
		return errors.New("la ruta no puede estar vacia")
	}

	// Resolver la ruta canonicamente para deshacer cualquier .. o symlink.
	ruta, err := filepath.EvalSymlinks(ruta)
	if err != nil {
		return fmt.Errorf("ruta inaccesible: %w", err)
	}
	ruta = filepath.Clean(ruta)

	if err := rutaEnZonaProhibida(ruta); err != nil {
		return err
	}

	info, err := os.Stat(ruta)
	if err != nil {
		return fmt.Errorf("fichero no encontrado: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("la ruta no apunta a un fichero regular")
	}
	return nil
}

// validarRutaEscritura comprueba que la ruta de salida es segura para escribir:
//   - El directorio padre existe y es escribible.
//   - No apunta a zonas de sistema prohibidas.
//   - El nombre no contiene caracteres de control.
func validarRutaEscritura(ruta string) error {
	if ruta == "" {
		return errors.New("la ruta de salida no puede estar vacia")
	}

	// Si el fichero ya existe, resolverlo.
	if _, err := os.Lstat(ruta); err == nil {
		rutaReal, err := filepath.EvalSymlinks(ruta)
		if err != nil {
			return fmt.Errorf("ruta de salida inaccesible: %w", err)
		}
		ruta = rutaReal
	}
	ruta = filepath.Clean(ruta)

	if err := rutaEnZonaProhibida(ruta); err != nil {
		return err
	}

	// Verificar que el directorio padre existe.
	dir := filepath.Dir(ruta)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("el directorio de salida no existe: %s", dir)
	}

	// Rechazar caracteres de control en el nombre del fichero.
	base := filepath.Base(ruta)
	for _, r := range base {
		if r < 32 {
			return errors.New("el nombre del fichero de salida contiene caracteres invalidos")
		}
	}
	return nil
}

func validarDirectorioLectura(ruta string) error {
	if ruta == "" {
		return errors.New("la carpeta no puede estar vacia")
	}
	rutaReal, err := filepath.EvalSymlinks(ruta)
	if err != nil {
		return fmt.Errorf("ruta inaccesible: %w", err)
	}
	rutaReal = filepath.Clean(rutaReal)
	if err := rutaEnZonaProhibida(rutaReal); err != nil {
		return err
	}
	info, err := os.Stat(rutaReal)
	if err != nil {
		return fmt.Errorf("carpeta no encontrada: %w", err)
	}
	if !info.IsDir() {
		return errors.New("la ruta no apunta a una carpeta")
	}
	return nil
}

func validarDirectorioEscritura(ruta string) error {
	if err := validarDirectorioLectura(ruta); err != nil {
		return err
	}
	base := filepath.Base(filepath.Clean(ruta))
	for _, r := range base {
		if r < 32 {
			return errors.New("el nombre de la carpeta contiene caracteres invalidos")
		}
	}
	return nil
}

// rutaEnZonaProhibida devuelve error si la ruta cae en alguno de los directorios
// de sistema que nunca debe tocar el IPC.
func rutaEnZonaProhibida(ruta string) error {
	for _, prefijo := range directoriosSistemaProhibidos {
		if ruta == prefijo || strings.HasPrefix(ruta, prefijo+"/") {
			return fmt.Errorf("acceso denegado: la ruta '%s' esta en una zona del sistema protegida", ruta)
		}
	}
	return nil
}

// verificarTamano comprueba que el fichero no supera maxTamanoFichero.
func verificarTamano(ruta string) error {
	info, err := os.Stat(ruta)
	if err != nil {
		return fmt.Errorf("no se pudo verificar el tamano de '%s': %w", ruta, err)
	}
	if info.Size() > maxTamanoFichero {
		return fmt.Errorf("el fichero supera el tamano maximo permitido (%d MB)", maxTamanoFichero/1024/1024)
	}
	return nil
}

func tamanoFicheroSeguroIPC(ruta string) (int64, error) {
	file, err := securefile.OpenRead(ruta)
	if err != nil {
		return 0, errors.New(
			"no se pudo inspeccionar uno de los documentos seleccionados",
		)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, errors.New(
			"no se pudo inspeccionar uno de los documentos seleccionados",
		)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 {
		return 0, errors.New(
			"uno de los documentos seleccionados no es un fichero regular",
		)
	}
	return info.Size(), nil
}

func leerFicheroSeguroIPC(ruta string) ([]byte, error) {
	return securefile.ReadFileLimit(ruta, maxTamanoFichero)
}

func esDirectorioRuta(ruta string) bool {
	info, err := os.Stat(strings.TrimSpace(ruta))
	return err == nil && info.IsDir()
}

// ---------------------------------------------------------------------------
// Helpers internos
// ---------------------------------------------------------------------------

func (m *Manejador) t(id string, args ...any) string {
	if m.Loc == nil {
		// Sin localizador devolvemos la clave tal cual; no usamos fmt.Sprintf
		// para que go vet no trate esta funcion como wrapper de printf.
		return id
	}
	return m.Loc.T(id, args...)
}

// resolverCertID devuelve el CertificateID del certificado en la posicion dada.
func (m *Manejador) resolverCertID(indice int) string {
	certs := m.snapshotUltimosCerts()
	if indice < 0 || indice >= len(certs) {
		if len(certs) == 1 {
			return certs[0].ID
		}
		return ""
	}
	return certs[indice].ID
}

// resolverCertIDPreferido da precedencia a certificateId cuando el cliente lo
// proporciona. certificateIndex sigue siendo el fallback para clientes legacy.
func (m *Manejador) resolverCertIDPreferido(ctx context.Context, certID string, indice int) (string, error) {
	certID = strings.TrimSpace(certID)
	if certID == "" {
		return m.resolverCertID(indice), nil
	}
	ref, ok, err := m.resolverCertRefPorID(ctx, certID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("no se ha encontrado el certificado seleccionado en el catálogo actual")
	}
	return ref.ID, nil
}

func (m *Manejador) resolverCertRefPorID(ctx context.Context, certID string) (domain.CertificateRef, bool, error) {
	for _, ref := range m.snapshotUltimosCerts() {
		if ref.ID == certID {
			return ref, true, nil
		}
	}
	if m.Catalogo == nil {
		return domain.CertificateRef{}, false, nil
	}
	refs, err := m.Catalogo.List(ctx)
	if err != nil {
		return domain.CertificateRef{}, false, err
	}
	m.setUltimosCerts(refs)
	for _, ref := range refs {
		if ref.ID == certID {
			return ref, true, nil
		}
	}
	return domain.CertificateRef{}, false, nil
}

func (m *Manejador) snapshotUltimosCerts() []domain.CertificateRef {
	m.certsMu.RLock()
	defer m.certsMu.RUnlock()
	return append([]domain.CertificateRef(nil), m.ultimosCerts...)
}

func (m *Manejador) setUltimosCerts(certs []domain.CertificateRef) {
	m.certsMu.Lock()
	defer m.certsMu.Unlock()
	m.ultimosCerts = append([]domain.CertificateRef(nil), certs...)
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func motivoNoAptoParaFirmar(reason signingpolicy.Reason) string {
	switch reason {
	case signingpolicy.DisallowedExtKeyUsage:
		return "No utilizable para firmar: solo sirve para servidores web u otros fines"
	case signingpolicy.CertificateAuthority:
		return "No utilizable para firmar: es de una autoridad de certificación"
	default:
		return "No utilizable para firmar: no permite firma digital"
	}
}

// certsAJSON convierte una lista de CertificateRef al formato JSON del cliente Qt.
// CanSign combina la vigencia con la capacidad no interactiva publicada por el
// catálogo. Nunca abre la clave durante un listado: hacerlo podría solicitar el
// PIN de una tarjeta antes de que el usuario haya pedido firmar.
func (m *Manejador) certsAJSON(certs []domain.CertificateRef) []certJSON {
	ahora := time.Now()
	items := make([]certJSON, 0, len(certs))
	for _, c := range certs {
		vigenciaDesconocida := c.NotAfter.IsZero()
		caducado := !vigenciaDesconocida && ahora.After(c.NotAfter)
		canSign := c.HasSigningKey && !vigenciaDesconocida && !caducado
		needsUnlock := c.SigningKeyNeedsUnlock && !c.HasSigningKey && !vigenciaDesconocida && !caducado
		// Misma política que aplica el firmante: un certificado solo de servidor
		// web o de autoridad se muestra, pero no se ofrece para firmar.
		noApto := ""
		if (canSign || needsUnlock) && len(c.DER) > 0 {
			var politica *signingpolicy.Error
			if err := signingpolicy.ValidateCertificateDER(c.DER, ahora); errors.As(err, &politica) &&
				(politica.Reason == signingpolicy.DisallowedExtKeyUsage ||
					politica.Reason == signingpolicy.CertificateAuthority ||
					politica.Reason == signingpolicy.DisallowedKeyUsage) {
				canSign, needsUnlock = false, false
				noApto = motivoNoAptoParaFirmar(politica.Reason)
			}
		}
		status := "Sin clave privada"
		switch {
		case noApto != "":
			status = noApto
		case caducado:
			status = "Caducado"
		case vigenciaDesconocida:
			status = "Vigencia desconocida"
		case canSign:
			status = "Válido"
		case needsUnlock:
			status = "Requiere autorización de la tarjeta"
		}
		item := certJSON{
			ID:           c.ID,
			Subject:      c.Subject,
			SubjectName:  c.Subject,
			Issuer:       c.Issuer,
			IssuerName:   c.Issuer,
			Fingerprint:  c.Fingerprint,
			SerialNumber: c.NIF,
			Status:       status,
			CanSign:      canSign,
			NeedsUnlock:  needsUnlock,
			Tipo:         string(c.Tipo),
			Organizacion: c.Organizacion,
			NIF:          c.NIF,
		}
		if string(c.Tipo) == "" {
			item.Tipo = string(domain.TipoCertDesconocido)
		}
		if !c.NotAfter.IsZero() {
			item.NotAfter = c.NotAfter.UTC().Format(time.RFC3339)
			item.ValidTo = item.NotAfter
			item.Caducado = caducado
			item.DiasCaducidad = int(c.NotAfter.Sub(ahora).Hours() / 24)
		}
		if m.CSC != nil {
			if remota, ok := m.CSC.Credencial(c.ID); ok {
				item.Remote = true
				item.RemoteMode = string(remota.Modo)
				item.RemotePIN = remota.PIN
				item.RemoteOTP = remota.OTP
				item.RemoteOTPOnline = remota.OTPEnLinea
				item.RemoteMultiSign = max(remota.Multisign, 1)
			}
		}
		items = append(items, item)
	}
	return items
}

// mimeDesdeExtension devuelve el MIME type segun la extension del fichero.
func mimeDesdeExtension(ruta string) string {
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".pdf":
		return "application/pdf"
	case ".dsig", ".xmlsig":
		return "application/xmldsig+xml"
	case ".xml":
		return "application/xml"
	case ".p7s", ".csig":
		return "application/pkcs7-signature"
	case ".xsig":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

func serializarHashCreadoIPC(resultado application.CreateHashResult) []byte {
	if resultado.Format == application.HashFormatBinary {
		return append([]byte(nil), resultado.Digest...)
	}
	return []byte(resultado.Encoded)
}

func construirRutaHashIPC(rutaEntrada string, format application.HashOutputFormat) string {
	switch format {
	case application.HashFormatBinary:
		return rutaEntrada + ".hash"
	case application.HashFormatBase64:
		return rutaEntrada + ".hashb64"
	default:
		return rutaEntrada + ".hexhash"
	}
}

func defaultDirectoryHashFilenameIPC(name string, format domain.DirectoryHashManifestFormat) string {
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

func defaultDirectoryHashReportFilenameIPC(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "directorio"
	}
	return base + ".hashreport"
}

func parseDirectoryHashRequestFormatIPC(raw string) (domain.DirectoryHashManifestFormat, error) {
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

func inferirFormatoRuta(ruta string) string {
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".pdf":
		return "pades"
	case ".asics":
		return "asic-xades"
	case ".odt", ".ods", ".odp", ".odg", ".odf":
		return "odf"
	case ".docx", ".xlsx", ".pptx", ".ppsx":
		return "ooxml"
	case ".dsig", ".xmlsig":
		return "xmldsig"
	case ".xml", ".xsig":
		return "xades"
	default:
		return "cades"
	}
}

func construirOpcionesFirmaIPC(params paramsFirma, formato string) (map[string]string, error) {
	return construirOpcionesFirmaIPCBase(
		params.AllowInvalidPDF,
		params.StrictCompat,
		params.VisibleSeal,
		params.QRContent,
		params.Reason,
		params.Location,
		params.ContactInfo,
		params.ExtraOptions,
		formato,
	)
}

func construirOpcionesFirmaLoteIPC(params paramsFirmaLote, formato string) (map[string]string, error) {
	return construirOpcionesFirmaIPCBase(
		params.AllowInvalidPDF,
		params.StrictCompat,
		params.VisibleSeal,
		params.QRContent,
		params.Reason,
		params.Location,
		params.ContactInfo,
		params.ExtraOptions,
		formato,
	)
}

func construirOpcionesFirmaLoteDocumentoIPC(params paramsFirmaLote, formato string, override *firmaLoteDocumentOverride) (map[string]string, error) {
	if override == nil {
		return construirOpcionesFirmaLoteIPC(params, formato)
	}
	itemParams := params
	itemParams.VisibleSeal = override.VisibleSeal
	return construirOpcionesFirmaLoteIPC(itemParams, formato)
}

func resolverOverridesFirmaLoteIPC(overrides []firmaLoteDocumentOverride, inputPaths []string) (map[string]*firmaLoteDocumentOverride, error) {
	if len(overrides) == 0 {
		return nil, nil
	}
	known := make(map[string]struct{}, len(inputPaths))
	for _, inputPath := range inputPaths {
		known[normalizarClaveRutaLote(inputPath)] = struct{}{}
	}
	resolved := make(map[string]*firmaLoteDocumentOverride, len(overrides))
	for i := range overrides {
		override := &overrides[i]
		if strings.TrimSpace(override.InputPath) == "" {
			return nil, errors.New("cada override del lote debe indicar inputPath")
		}
		key := normalizarClaveRutaLote(override.InputPath)
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("el override del lote no corresponde a un fichero seleccionado: %s", filepath.Base(override.InputPath))
		}
		if _, duplicate := resolved[key]; duplicate {
			return nil, fmt.Errorf("override duplicado para el fichero: %s", filepath.Base(override.InputPath))
		}
		if len(override.VisibleSeal) == 0 {
			return nil, fmt.Errorf("el override de %s no contiene un sello visible", filepath.Base(override.InputPath))
		}
		resolved[key] = override
	}
	return resolved, nil
}

func normalizarClaveRutaLote(path string) string {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		absolute = filepath.Clean(strings.TrimSpace(path))
	}
	absolute = filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		return strings.ToLower(absolute)
	}
	return absolute
}

func construirOpcionesFirmaIPCBase(allowInvalidPDF, strictCompat bool, visibleSeal map[string]any, qrContent, reason, location, contactInfo string, extraOptions map[string]string, formato string) (map[string]string, error) {
	out := map[string]string{}
	if allowInvalidPDF {
		out["allowInvalidPDF"] = "true"
	}
	if strictCompat {
		out["strictCompat"] = "true"
		if strings.EqualFold(formato, "pades") {
			out["subfilter"] = "adbe.pkcs7.detached"
		}
	}
	if strings.EqualFold(formato, "pades") && len(visibleSeal) > 0 {
		out["visibleSeal"] = "true"
		if raw, present := visibleSeal["placements"]; present {
			entries, ok := raw.([]any)
			if !ok || len(entries) == 0 || len(entries) > 128 {
				return nil, fmt.Errorf("la lista de sellos debe tener entre 1 y 128 páginas")
			}
			encoded, err := json.Marshal(entries)
			if err != nil || len(encoded) > 32*1024 {
				return nil, fmt.Errorf("la lista de sellos no es válida")
			}
			out["visibleSealPlacements"] = string(encoded)
		}
		pageWidth := valorFloat(visibleSeal["pageWidth"], 0)
		pageHeight := valorFloat(visibleSeal["pageHeight"], 0)
		if pageWidth <= 0 || pageHeight <= 0 ||
			math.IsNaN(pageWidth) || math.IsNaN(pageHeight) ||
			math.IsInf(pageWidth, 0) || math.IsInf(pageHeight, 0) {
			return nil, fmt.Errorf("las dimensiones reales de la página PDF son obligatorias para el sello visible")
		}
		x := clamp01(valorFloat(visibleSeal["x"], 0.62))
		y := clamp01(valorFloat(visibleSeal["y"], 0.04))
		w := clamp01(valorFloat(visibleSeal["w"], 0.34))
		h := clamp01(valorFloat(visibleSeal["h"], 0.12))
		out["visibleSealRectX"] = fmt.Sprintf("%.2f", x*pageWidth)
		out["visibleSealRectY"] = fmt.Sprintf("%.2f", y*pageHeight)
		out["visibleSealRectW"] = fmt.Sprintf("%.2f", w*pageWidth)
		out["visibleSealRectH"] = fmt.Sprintf("%.2f", h*pageHeight)
		out["visibleSealPageWidth"] = fmt.Sprintf("%.2f", pageWidth)
		out["visibleSealPageHeight"] = fmt.Sprintf("%.2f", pageHeight)
		out["visibleSealRectRelativeToCrop"] = "true"
		out["page"] = valorPaginaVisibleSeal(visibleSeal["page"], 1)
		out["rotation"] = fmt.Sprintf("%d", valorInt(visibleSeal["rotation"], 0))
		if keepText, ok := visibleSeal["keepText"]; ok {
			out["visibleSealKeepText"] = fmt.Sprintf("%t", valorBool(keepText, true))
		}
		if imagePath := strings.TrimSpace(valorTexto(visibleSeal["imagePath"])); imagePath != "" {
			out["visibleSealImagePath"] = imagePath
		}
		if raw, present := visibleSeal["logoOpacityPercent"]; present {
			opacity, ok := porcentajeOpacidadSelloIPC(raw)
			if !ok {
				return nil, ports.ErrOpacidadLogoSello
			}
			out["visibleSealLogoOpacityPercent"] = strconv.Itoa(opacity)
		}
	}
	if strings.EqualFold(formato, "pades") {
		if qr := strings.TrimSpace(qrContent); qr != "" {
			out["qrContent"] = qr
		}
		if reason := strings.TrimSpace(reason); reason != "" {
			out["reason"] = reason
		}
		if location := strings.TrimSpace(location); location != "" {
			out["location"] = location
		}
		if contact := strings.TrimSpace(contactInfo); contact != "" {
			out["contactInfo"] = contact
		}
	}
	seenOpacityOption := false
	for k, v := range extraOptions {
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		if strings.EqualFold(key, "visibleSealLogoOpacityPercent") {
			if seenOpacityOption {
				return nil, ports.ErrOpacidadLogoSello
			}
			seenOpacityOption = true
			opacity, err := strconv.Atoi(value)
			if err != nil || opacity < 0 || opacity > 100 {
				return nil, ports.ErrOpacidadLogoSello
			}
			out["visibleSealLogoOpacityPercent"] = strconv.Itoa(opacity)
			continue
		}
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil, nil
	}
	if err := materializarImagenSelloIPC(out); err != nil {
		return nil, err
	}
	return out, nil
}

func porcentajeOpacidadSelloIPC(raw any) (int, bool) {
	var value int
	switch n := raw.(type) {
	case int:
		value = n
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < 0 || n > 100 {
			return 0, false
		}
		value = int(n)
	default:
		return 0, false
	}
	return value, value >= 0 && value <= 100
}

func (m *Manejador) localizarErrorOpacidadLogoSello(err error) string {
	if errors.Is(err, ports.ErrOpacidadLogoSello) {
		return m.t("error.opacidad_logo_sello")
	}
	return err.Error()
}

func materializarImagenSelloIPC(options map[string]string) error {
	var imagePath string
	for key, value := range options {
		if strings.EqualFold(strings.TrimSpace(key), "visibleSealImagePath") {
			imagePath = strings.TrimSpace(value)
			delete(options, key)
		}
	}
	if imagePath == "" {
		return nil
	}
	if err := validarRutaLectura(imagePath); err != nil {
		return fmt.Errorf("ruta de imagen de sello no válida: %w", err)
	}
	data, err := securefile.ReadFileLimit(imagePath, 10*1024*1024)
	if err != nil {
		return fmt.Errorf("no se pudo leer la imagen de sello: %w", err)
	}
	options["visibleSealImageBase64"] = base64.StdEncoding.EncodeToString(data)
	return nil
}

func valorPaginaVisibleSeal(v any, fallback int) string {
	switch page := v.(type) {
	case string:
		page = strings.TrimSpace(page)
		if page == "" {
			break
		}
		switch strings.ToLower(page) {
		case "all", "*", "todas", "todos":
			return "all"
		}
		return page
	}
	return fmt.Sprintf("%d", valorInt(v, fallback))
}

func valorFloat(v any, fallback float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	}
	return fallback
}

func valorInt(v any, fallback int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return fallback
}

func valorBool(v any, fallback bool) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		switch strings.TrimSpace(strings.ToLower(b)) {
		case "true", "1", "si", "sí", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}

func valorTexto(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	default:
		return ""
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// rutaSalida calcula la ruta de salida para el fichero firmado.
func rutaSalida(inputPath, formato, overwrite string) string {
	dir := filepath.Dir(inputPath)
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), ext)
	sufijo := sufijoSalidaFirma(formato)
	salida := filepath.Join(dir, base+sufijo)

	if overwrite == "overwrite" || overwrite == "force" {
		return salida
	}
	if _, err := os.Stat(salida); os.IsNotExist(err) {
		return salida
	}
	for i := 2; i < 100; i++ {
		candidato := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, sufijo))
		if _, err := os.Stat(candidato); os.IsNotExist(err) {
			return candidato
		}
	}
	return salida
}

func reservarRutaSalidaLote(
	inputPath,
	outputDir,
	formato,
	overwrite string,
	reserved map[string]struct{},
) (string, error) {
	baseInput := inputPath
	if outputDir != "" {
		baseInput = filepath.Join(outputDir, filepath.Base(inputPath))
	}
	dir := filepath.Dir(baseInput)
	ext := filepath.Ext(baseInput)
	base := strings.TrimSuffix(filepath.Base(baseInput), ext)
	suffix := sufijoSalidaFirma(formato)
	candidate := filepath.Join(dir, base+suffix)

	if overwrite == "overwrite" || overwrite == "force" {
		key := normalizarClaveRutaLote(candidate)
		if _, collision := reserved[key]; collision {
			return "", errors.New(
				"varios documentos del lote producirían la misma ruta de salida",
			)
		}
		reserved[key] = struct{}{}
		return candidate, nil
	}

	for index := 1; index <= maxIPCBatchDocuments+1; index++ {
		if index > 1 {
			candidate = filepath.Join(
				dir,
				fmt.Sprintf("%s_%d%s", base, index, suffix),
			)
		}
		key := normalizarClaveRutaLote(candidate)
		if _, collision := reserved[key]; collision {
			continue
		}
		if _, err := os.Lstat(candidate); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", errors.New(
				"no se pudo comprobar una ruta de salida del lote",
			)
		}
		reserved[key] = struct{}{}
		return candidate, nil
	}
	return "", errors.New(
		"no se pudo reservar una ruta de salida única para el lote",
	)
}

func sufijoSalidaFirma(formato string) string {
	switch strings.ToLower(formato) {
	case "pades":
		return "_firmado.pdf"
	case "verifactu":
		return "_firmado.xml"
	case "xmldsig":
		return "_firmado.dsig"
	case "xades":
		return "_firmado.xsig"
	default:
		return "_firmado.p7s"
	}
}

func recogerRutasFirmaLote(inputPaths []string, directoryPath string) ([]string, error) {
	out := make([]string, 0, len(inputPaths))
	seen := make(map[string]struct{}, len(inputPaths))
	addPath := func(path string) error {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil
		}
		key := normalizarClaveRutaLote(path)
		if _, ok := seen[key]; ok {
			return nil
		}
		if len(out) >= maxIPCBatchDocuments {
			return fmt.Errorf(
				"el lote supera el máximo permitido de %d documentos",
				maxIPCBatchDocuments,
			)
		}
		seen[key] = struct{}{}
		out = append(out, path)
		return nil
	}

	for _, inputPath := range inputPaths {
		if err := addPath(inputPath); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(directoryPath) == "" {
		return out, nil
	}
	if err := validarDirectorioLectura(directoryPath); err != nil {
		return nil, err
	}
	dir, err := securefile.OpenDir(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la carpeta seleccionada: %w", err)
	}
	defer dir.Close()
	for {
		entries, readErr := dir.ReadDir(64)
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil || !info.Mode().IsRegular() {
				continue
			}
			if err := addPath(filepath.Join(directoryPath, entry.Name())); err != nil {
				return nil, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf(
				"no se pudo leer la carpeta seleccionada: %w",
				readErr,
			)
		}
	}
	return out, nil
}
