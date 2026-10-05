// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"encoding/json"
	"strings"
)

// peticion es el JSON que envía la GUI Qt (IpcBridge::sendRequest).
// Protocolo: una linea JSON terminada en \n por peticion.
//
//	{"action":"sign","params":{...}}
type peticion struct {
	Protocol  string          `json:"protocol,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
	TraceID   string          `json:"traceId,omitempty"`
	Action    string          `json:"action"`
	Params    json.RawMessage `json:"params"`

	// Estos indicadores permiten distinguir clientes legacy que omiten los
	// IDs de entradas presentes pero vacías o null, que deben rechazarse.
	protocolPresent  bool
	requestIDPresent bool
	traceIDPresent   bool
	paramsPresent    bool
	paramsNull       bool
}

func (p *peticion) UnmarshalJSON(data []byte) error {
	type wirePeticion peticion

	var decoded wirePeticion
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if isLocalTokenSettingsAction(decoded.Action) {
		if err := validateTokenSettingsEnvelope(data); err != nil {
			clear(decoded.Params)
			return err
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	*p = peticion(decoded)
	for name := range fields {
		if strings.EqualFold(name, "protocol") {
			p.protocolPresent = true
		}
		if strings.EqualFold(name, "requestId") {
			p.requestIDPresent = true
		}
		if strings.EqualFold(name, "traceId") {
			p.traceIDPresent = true
		}
		if strings.EqualFold(name, "params") {
			p.paramsPresent = true
			p.paramsNull = strings.TrimSpace(string(fields[name])) == "null"
		}
	}
	return nil
}

// respuesta es el JSON que devuelve el servidor al cliente Qt (onReadyRead).
// Protocolo: una linea JSON terminada en \n por respuesta.
//
//	{"ok":true,"action":"sign","data":{...}}
//	{"ok":false,"action":"sign","error":"mensaje localizado"}
type respuesta struct {
	Protocol  string `json:"protocol,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
	OK        bool   `json:"ok"`
	Action    string `json:"action"`
	// ErrorCode, Phase, Retryable y Outcome forman el contrato estable que
	// permite a las GUI representar un resultado sin interpretar Error.
	ErrorCode string `json:"errorCode"`
	Phase     string `json:"phase"`
	Retryable bool   `json:"retryable"`
	Outcome   string `json:"outcome"`
	// Data puede ser cualquier valor JSON (objeto, array, string, numero).
	Data       any                         `json:"data,omitempty"`
	Error      string                      `json:"error,omitempty"`
	Diagnostic *resultadoDiagnosticoGuiado `json:"diagnostic,omitempty"`
}

// paramsFirma mapea los parametros de la accion "sign".
type paramsFirma struct {
	InputPath                string            `json:"inputPath"`
	OutputPath               string            `json:"outputPath"`
	CertificateID            string            `json:"certificateId,omitempty"`
	CertificateIndex         int               `json:"certificateIndex"`
	AdditionalCertificateIDs []string          `json:"additionalCertificateIds,omitempty"`
	Format                   string            `json:"format"`
	Action                   string            `json:"action"`
	Overwrite                string            `json:"overwrite"`
	SaveToDisk               bool              `json:"saveToDisk"`
	ReturnSignatureB64       bool              `json:"returnSignatureB64"`
	VisibleSeal              map[string]any    `json:"visibleSeal,omitempty"`
	AllowInvalidPDF          bool              `json:"allowInvalidPDF"`
	StrictCompat             bool              `json:"strictCompat"`
	QRContent                string            `json:"qrContent,omitempty"`
	Reason                   string            `json:"reason,omitempty"`
	Location                 string            `json:"location,omitempty"`
	ContactInfo              string            `json:"contactInfo,omitempty"`
	ExtraOptions             map[string]string `json:"extraOptions,omitempty"`
}

// paramsFirmaLote mapea los parametros de la accion "sign_batch".
type paramsFirmaLote struct {
	InputPaths               []string                    `json:"inputPaths,omitempty"`
	DirectoryPath            string                      `json:"directoryPath,omitempty"`
	OutputDir                string                      `json:"outputDir,omitempty"`
	CertificateID            string                      `json:"certificateId,omitempty"`
	CertificateIndex         int                         `json:"certificateIndex"`
	AdditionalCertificateIDs []string                    `json:"additionalCertificateIds,omitempty"`
	Format                   string                      `json:"format"`
	Action                   string                      `json:"action"`
	Overwrite                string                      `json:"overwrite"`
	VisibleSeal              map[string]any              `json:"visibleSeal,omitempty"`
	AllowInvalidPDF          bool                        `json:"allowInvalidPDF"`
	StrictCompat             bool                        `json:"strictCompat"`
	QRContent                string                      `json:"qrContent,omitempty"`
	Reason                   string                      `json:"reason,omitempty"`
	Location                 string                      `json:"location,omitempty"`
	ContactInfo              string                      `json:"contactInfo,omitempty"`
	ExtraOptions             map[string]string           `json:"extraOptions,omitempty"`
	DocumentOverrides        []firmaLoteDocumentOverride `json:"documentOverrides,omitempty"`
}

// firmaLoteDocumentOverride permite que la GUI sobrescriba el sello visible
// de un fichero concreto sin alterar la plantilla global del lote.
type firmaLoteDocumentOverride struct {
	InputPath   string         `json:"inputPath"`
	VisibleSeal map[string]any `json:"visibleSeal"`
}

// paramsVerify mapea los parametros de la accion "verify".
type paramsVerify struct {
	InputPath    string `json:"inputPath"`
	OriginalPath string `json:"originalPath,omitempty"`
}

type paramsCertificateOnlineCheck struct {
	CertificateID string `json:"certificateId"`
}

type paramsProtection struct {
	InputPath            string            `json:"inputPath"`
	OutputPath           string            `json:"outputPath,omitempty"`
	CertificateID        string            `json:"certificateId,omitempty"`
	CertificateIndex     int               `json:"certificateIndex,omitempty"`
	Profile              string            `json:"profile"`
	RecipientIDs         []string          `json:"recipientIds,omitempty"`
	Overwrite            string            `json:"overwrite,omitempty"`
	SaveToDisk           bool              `json:"saveToDisk"`
	ReturnProtectedB64   bool              `json:"returnProtectedB64,omitempty"`
	ReturnUnprotectedB64 bool              `json:"returnUnprotectedB64,omitempty"`
	Options              map[string]string `json:"options,omitempty"`
	// SecretB64 recibe directamente los 32 bytes que JSON representa en
	// Base64. Evita materializar la clave de EncryptedData como string.
	SecretB64 *[]byte `json:"secretB64,omitempty"`
}

type paramsHashCreate struct {
	InputPath  string `json:"inputPath"`
	OutputPath string `json:"outputPath"`
	Algorithm  string `json:"algorithm"`
	Format     string `json:"format"`
	Recursive  bool   `json:"recursive"`
}

type paramsHashCheck struct {
	InputPath        string `json:"inputPath"`
	HashPath         string `json:"hashPath"`
	OutputPath       string `json:"outputPath,omitempty"`
	Algorithm        string `json:"algorithm,omitempty"`
	Recursive        bool   `json:"recursive,omitempty"`
	SaveReportToDisk bool   `json:"saveReportToDisk,omitempty"`
}

// paramsPdfPreview mapea los parametros de la accion "pdf_preview".
type paramsPdfPreview struct {
	Path string `json:"path"`
	Page int    `json:"page"`
}

// paramsInstallService mapea los parametros de "service_install".
type paramsInstallService struct {
	IpcSocket string `json:"ipcSocket"`
}

// paramsImportCert mapea los parametros de "import_certificate".
type paramsImportCert struct {
	P12B64      string  `json:"p12B64"`
	Password    string  `json:"password,omitempty"`
	PasswordB64 *[]byte `json:"passwordB64,omitempty"`
}

type paramsOpenCertificateManager struct {
	ManagerID string `json:"managerId"`
}

type paramsImportCertificateToStore struct {
	CredentialB64 []byte  `json:"credentialB64"`
	Password      string  `json:"password,omitempty"`
	PasswordB64   *[]byte `json:"passwordB64,omitempty"`
	TargetID      string  `json:"targetId"`
}

type paramsUseTemporaryCertificate struct {
	CredentialB64 []byte  `json:"credentialB64"`
	Password      string  `json:"password,omitempty"`
	PasswordB64   *[]byte `json:"passwordB64,omitempty"`
}

type paramsRemoveTemporaryCertificate struct {
	CertificateID string `json:"certificateId"`
}

type certificateAccessManagerJSON struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Recommended bool   `json:"recommended"`
}

type certificateImportTargetJSON struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Browser     string `json:"browser,omitempty"`
	Recommended bool   `json:"recommended"`
}

type certificateAccessOptionsJSON struct {
	Managers         []certificateAccessManagerJSON `json:"managers"`
	ImportTargets    []certificateImportTargetJSON  `json:"importTargets"`
	DetectedBrowser  string                         `json:"detectedBrowser,omitempty"`
	PreferredManager string                         `json:"preferredManager,omitempty"`
	PreferredTarget  string                         `json:"preferredTarget,omitempty"`
}

type temporaryCertificateJSON struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Fingerprint string `json:"fingerprint"`
	Temporary   bool   `json:"temporary"`
}

// certJSON es la representacion JSON de un certificado para el cliente Qt.
// Los campos extra (tipo, organizacion, nif, caducado, diasCaducidad) permiten
// que la GUI filtre y muestre certificados segun el contexto de firma.
type certJSON struct {
	ID           string `json:"id"`
	Subject      string `json:"subject"`
	SubjectName  string `json:"subjectName"`
	Issuer       string `json:"issuer"`
	IssuerName   string `json:"issuerName"`
	NotAfter     string `json:"notAfter,omitempty"`
	ValidTo      string `json:"validTo,omitempty"`
	Fingerprint  string `json:"fingerprint"`
	SerialNumber string `json:"serialNumber,omitempty"`
	Status       string `json:"status,omitempty"`
	CanSign      bool   `json:"canSign"`
	NeedsUnlock  bool   `json:"needsUnlock"` // requiere autorización del token; no garantiza clave disponible
	// Campos de clasificacion
	Tipo          string `json:"tipo"`                   // "fisica", "representacion", "sello", "empleado_publico", "desconocido"
	Organizacion  string `json:"organizacion,omitempty"` // nombre de la empresa (si aplica)
	NIF           string `json:"nif,omitempty"`          // numero de identificacion (ETSI EN 319 412-1)
	Caducado      bool   `json:"caducado"`               // true si NotAfter < ahora
	DiasCaducidad int    `json:"diasCaducidad"`          // dias restantes (negativo si ya caducó)
	// Firma remota CSC: el certificado lo custodia un prestador. Indica qué
	// datos debe pedir la interfaz antes de firmar; nunca lleva tokens.
	Remote          bool   `json:"remote,omitempty"`
	RemoteMode      string `json:"remoteMode,omitempty"`
	RemotePIN       bool   `json:"remotePin,omitempty"`
	RemoteOTP       bool   `json:"remoteOtp,omitempty"`
	RemoteOTPOnline bool   `json:"remoteOtpOnline,omitempty"`
	// RemoteMultiSign es cuántas firmas autoriza el prestador de una vez;
	// con 2 o más, un lote con OTP pide un solo código si cabe.
	RemoteMultiSign int `json:"remoteMultiSign,omitempty"`
}

// resultadoFirma es el objeto data de la respuesta "sign".
type resultadoFirma struct {
	Format       string `json:"format,omitempty"`
	OutputPath   string `json:"OutputPath,omitempty"`
	SignatureB64 string `json:"signature_b64,omitempty"`
}

type resultadoFirmaLoteItem struct {
	InputPath  string `json:"inputPath"`
	OutputPath string `json:"outputPath,omitempty"`
	Format     string `json:"format,omitempty"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

type resultadoFirmaLote struct {
	Results   []resultadoFirmaLoteItem `json:"results"`
	OkCount   int                      `json:"okCount"`
	FailCount int                      `json:"failCount"`
}

// resultadoVerificacion es el objeto data de la respuesta "verify".
type resultadoVerificacion struct {
	Valid           bool                             `json:"valid"`
	Reason          string                           `json:"reason,omitempty"`
	Details         []string                         `json:"details,omitempty"`
	Signers         []string                         `json:"signers,omitempty"`
	Format          string                           `json:"format,omitempty"`
	Coverage        string                           `json:"coverage,omitempty"`
	Integrity       resultadoVerificacionAspecto     `json:"integrity"`
	Certificate     resultadoVerificacionAspecto     `json:"certificate"`
	Trust           resultadoVerificacionAspecto     `json:"trust"`
	SignerSummaries []resultadoVerificacionFirmante  `json:"signerSummaries,omitempty"`
	Warnings        []string                         `json:"warnings,omitempty"`
	Errors          []string                         `json:"errors,omitempty"`
	Evidence        []resultadoVerificacionEvidencia `json:"evidence,omitempty"`
}

type resultadoVerificacionAspecto struct {
	Status  string   `json:"status"`
	Reason  string   `json:"reason,omitempty"`
	Details []string `json:"details,omitempty"`
}

type resultadoVerificacionFirmante struct {
	ID          string `json:"id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type resultadoVerificacionEvidencia struct {
	Type    string `json:"type"`
	Summary string `json:"summary,omitempty"`
}

type resultadoEstadoCertificadoOnline struct {
	Status      string `json:"status"`
	UserMessage string `json:"userMessage,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Method      string `json:"method,omitempty"`
	CheckedAt   string `json:"checkedAt,omitempty"`
	RevokedAt   string `json:"revokedAt,omitempty"`
	OCSPURL     string `json:"ocspUrl,omitempty"`
	CRLURL      string `json:"crlUrl,omitempty"`
}

type destinatarioProteccionJSON struct {
	ID                          string `json:"id"`
	Label                       string `json:"label"`
	Origin                      string `json:"origin"`
	Profile                     string `json:"profile"`
	Algorithm                   string `json:"algorithm,omitempty"`
	AuthEnvelopedDataCompatible bool   `json:"authEnvelopedDataCompatible"`
}

type resultadoDestinatariosProteccion struct {
	Recipients []destinatarioProteccionJSON `json:"recipients"`
}

type resultadoProteccion struct {
	OutputPath          string `json:"outputPath,omitempty"`
	ProtectedContentB64 string `json:"protectedContentBase64,omitempty"`
	Profile             string `json:"profile,omitempty"`
	RecipientCount      int    `json:"recipientCount,omitempty"`
	DocumentName        string `json:"documentName,omitempty"`
	MIMEType            string `json:"mimeType,omitempty"`
	CertificateID       string `json:"certificateId,omitempty"`
}

type resultadoDesproteccion struct {
	OutputPath            string `json:"outputPath,omitempty"`
	UnprotectedContentB64 string `json:"unprotectedContentBase64,omitempty"`
	Profile               string `json:"profile,omitempty"`
	RecipientID           string `json:"recipientId,omitempty"`
	DocumentName          string `json:"documentName,omitempty"`
	MIMEType              string `json:"mimeType,omitempty"`
}

type resultadoHash struct {
	Algorithm  string `json:"algorithm"`
	Format     string `json:"format"`
	Hash       string `json:"hash,omitempty"`
	OutputPath string `json:"outputPath,omitempty"`
}

type resultadoHashDirectorio struct {
	Algorithm   string `json:"algorithm"`
	Format      string `json:"format"`
	Entries     int    `json:"entries"`
	Recursive   bool   `json:"recursive"`
	OutputPath  string `json:"outputPath,omitempty"`
	ManifestB64 string `json:"manifestBase64,omitempty"`
}

type resultadoComprobarHash struct {
	Valid        bool   `json:"valid"`
	Algorithm    string `json:"algorithm"`
	Format       string `json:"format,omitempty"`
	ExpectedHash string `json:"expectedHash,omitempty"`
	ActualHash   string `json:"actualHash,omitempty"`
}

type resultadoComprobarHashDirectorio struct {
	Valid            bool     `json:"valid"`
	Algorithm        string   `json:"algorithm"`
	Recursive        bool     `json:"recursive"`
	MatchingHash     []string `json:"matching_hash,omitempty"`
	NotMatchingHash  []string `json:"not_matching_hash,omitempty"`
	HashWithoutFile  []string `json:"hash_without_file,omitempty"`
	FileWithoutHash  []string `json:"file_without_hash,omitempty"`
	ReportB64        string   `json:"reportBase64,omitempty"`
	ReportOutputPath string   `json:"reportOutputPath,omitempty"`
}

// resultadoPdfPreview es el objeto data de la respuesta "pdf_preview".
type resultadoPdfPreview struct {
	Data        string  `json:"data"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	CurrentPage int     `json:"currentPage"`
	TotalPages  int     `json:"totalPages"`
}

// resultadoEstadoServicio es el objeto data de la respuesta "service_status".
type resultadoEstadoServicio struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Platform  string `json:"platform"`
	Method    string `json:"method"`
}

type resultadoEstadoProxySecretStore struct {
	Available   bool   `json:"available"`
	Platform    string `json:"platform,omitempty"`
	Backend     string `json:"backend"`
	Reason      string `json:"reason,omitempty"`
	RuntimeMode string `json:"runtimeProxyMode,omitempty"`
}

type paramsProxySecretStore struct {
	Realm    string `json:"realm"`
	Username string `json:"username"`
	// encoding/json decodifica una cadena Base64 directamente en []byte, lo
	// que permite zeroizar el material tras entregarlo al almacén seguro.
	Password []byte `json:"password"`
}

type resultadoCredencialProxy struct {
	Configured bool   `json:"configured"`
	Realm      string `json:"realm,omitempty"`
	Username   string `json:"username,omitempty"`
	Rotated    bool   `json:"rotated,omitempty"`
}

// resultadoCheckCerts es el objeto data de la respuesta "check_certificates".
type resultadoCheckCerts struct {
	Certificates []certJSON `json:"certificates"`
	OkCount      int        `json:"okCount"`
	FailCount    int        `json:"failCount"`
}

// resultadoDiagnostico es el objeto data de la respuesta "export_diagnostic".
type resultadoDiagnostico struct {
	Certificates int                   `json:"certificates"`
	CanSign      int                   `json:"canSign"`
	TLSStore     resultadoEstadoTLSIPC `json:"tlsStore"`
}

// resultadoEstadoTLSIPC expone únicamente estados y recuentos. Las rutas y los
// nombres de los artefactos locales no forman parte del contrato de diagnóstico.
type resultadoEstadoTLSIPC struct {
	State            string `json:"state"`
	ArtifactCount    int    `json:"artifactCount"`
	CertificateCount int    `json:"certificateCount"`
	KeyCount         int    `json:"keyCount"`
}

type resultadoDiagnosticoGuiado struct {
	Category               string                  `json:"category"`
	FailureCode            string                  `json:"failureCode,omitempty"`
	UserMessage            string                  `json:"userMessage"`
	ExpertMessage          string                  `json:"expertMessage"`
	LikelyOwner            string                  `json:"likelyOwner,omitempty"`
	ResponsibilityMessage  string                  `json:"responsibilityMessage,omitempty"`
	SuggestedAction        string                  `json:"suggestedAction,omitempty"`
	UserCanResolveDirectly bool                    `json:"userCanResolveDirectly,omitempty"`
	Steps                  []pasoDiagnosticoGuiado `json:"steps,omitempty"`
}

// pasoDiagnosticoGuiado describe evidencia ya observada durante la operación.
// No inicia sondas ni presupone que una fase no registrada haya ocurrido.
type pasoDiagnosticoGuiado struct {
	Code            string `json:"code"`
	Label           string `json:"label"`
	Status          string `json:"status"`
	Owner           string `json:"owner,omitempty"`
	UserMessage     string `json:"userMessage,omitempty"`
	SuggestedAction string `json:"suggestedAction,omitempty"`
	EvidenceRef     string `json:"evidenceRef,omitempty"`
}
