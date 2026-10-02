// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"time"
)

// SignResult es el resultado de una operacion de firma completada con exito.
type SignResult struct {
	// Result contiene los datos firmados y los metadatos de la firma.
	Result domain.SignatureResult

	// CertificateUsed es el certificado que se uso para firmar.
	CertificateUsed domain.CertificateRef

	// CertificateChainDER conserva una copia de la cadena empleada, en orden
	// hoja -> emisores. No contiene claves privadas ni referencias al almacén.
	CertificateChainDER [][]byte
}

// SelectCertificateResult es el resultado de la seleccion de certificado por el usuario.
type SelectCertificateResult struct {
	// Selection contiene el certificado elegido y la confirmacion del usuario.
	Selection domain.CertificateSelection
}

// BatchResult es el resultado del procesado de un lote de firmas.
type BatchResult struct {
	// Results contiene los resultados individuales de cada trabajo del lote,
	// en el mismo orden que los trabajos del comando original.
	Results []SignResult

	// Errores contiene los errores de los trabajos que fallaron,
	// indexados por posicion. Puede estar vacio si todos tuvieron exito.
	Errores map[int]error
}

func (b BatchResult) TodosExitosos() bool {
	return len(b.Errores) == 0
}

// VerifyResult es el resultado de la verificacion de un documento firmado.
type VerifyResult struct {
	// Verification contiene el resultado tecnico de la verificacion.
	Verification domain.VerificationResult

	// Firmantes contiene la lista de certificados firmantes encontrados.
	Firmantes []domain.CertificateRef

	// Dictamen es el resultado explícito de la evaluación autónoma. Es nil
	// cuando no hay evaluador configurado.
	Dictamen *domain.DictamenVerificacion
}

// ProtectResult resume la protección/cifrado de un documento.
type ProtectResult struct {
	Protected domain.ProtectedPayload
}

// ProtectAndSignResult resume la operación de proteger firmando.
type ProtectAndSignResult struct {
	Protected       domain.ProtectedPayload
	CertificateUsed domain.CertificateRef
}

// UnprotectResult resume el descifrado correcto de un documento protegido.
type UnprotectResult struct {
	Unprotected domain.UnprotectedPayload
}

type CreateHashResult struct {
	Algorithm string
	Format    HashOutputFormat
	Digest    []byte
	Encoded   string
}

type CheckHashResult struct {
	Valid           bool
	Algorithm       string
	Format          HashOutputFormat
	ExpectedDigest  []byte
	ActualDigest    []byte
	ExpectedEncoded string
	ActualEncoded   string
}

type CreateDirectoryHashManifestResult struct {
	Algorithm string
	Format    domain.DirectoryHashManifestFormat
	Manifest  domain.DirectoryHashManifest
	Data      []byte
}

type CheckDirectoryHashManifestResult struct {
	Valid  bool
	Report domain.DirectoryHashCheckReport
}

// ExportProtectionRecipientResult resume la exportación del material público
// de un destinatario de protección.
type ExportProtectionRecipientResult struct {
	Recipient domain.ProtectionRecipient
	Data      []byte
}

// ImportProtectionRecipientResult resume la importación correcta de un
// destinatario público de protección.
type ImportProtectionRecipientResult struct {
	Recipient domain.ProtectionRecipient
}

// RetrieveRequestResult es el resultado de recuperar una peticion remota pendiente.
type RetrieveRequestResult struct {
	// Session identifica la sesion asociada a la peticion recuperada.
	Session domain.ExchangeSession

	// Data contiene la carga util remota tal y como la entrega el transporte.
	Data []byte
}

// ResolvePlatformProfileResult describe las capacidades observadas
// en la plataforma actual.
type ResolvePlatformProfileResult struct {
	Profile ports.CapabilityProfile
}

// NotifyUserResult resume el canal por el que se entrego la notificacion.
type NotifyUserResult struct {
	Channel string
}

// ManageTrustedDomainResult resume la operacion aplicada sobre un origen.
type ManageTrustedDomainResult struct {
	Origin string
	Action TrustAction
}

// ImportCertificateResult resume el certificado importado correctamente.
type ImportCertificateResult struct {
	Certificate domain.CertificateRef
}

type ListCertificateAccessOptionsResult struct {
	Options ports.CertificateAccessOptions
}

type UseTemporaryCertificateResult struct {
	Certificate domain.CertificateRef
}

// AuditRecord es una evidencia saneada lista para ser persistida por EvidenceLogger.
// Solo contiene campos que han pasado por la politica de sanitizacion de AuditOperation.
// Nunca contiene payloads completos, valor-pruebas ni material criptografico.
type AuditRecord struct {
	// Timestamp es el momento en que se registro la operacion.
	Timestamp time.Time `json:"timestamp"`

	// OperationType describe el tipo de operacion.
	OperationType string `json:"operacion"`

	// Origin es el origen sanitizado de la solicitud.
	Origin string `json:"origen,omitempty"`

	// CertificateFingerprint es la huella del certificado usado, si aplica.
	// Se usa la huella en lugar del certificado completo.
	CertificateFingerprint string `json:"cert_fingerprint,omitempty"`

	// DocumentName es el nombre del documento afectado, si aplica.
	DocumentName string `json:"documento,omitempty"`

	// DocumentHash es el SHA-256 hexadecimal del documento o payload relevante.
	DocumentHash string `json:"documento_sha256,omitempty"`

	// Format indica el formato funcional de la operacion si aplica.
	Format string `json:"formato,omitempty"`

	// Success indica si la operacion fue exitosa.
	Success bool `json:"exito"`

	// Result serializa el estado en forma legible para operadores: ok o error.
	Result string `json:"resultado"`

	// ErrorSummary es un resumen del error, ya sanitizado.
	ErrorSummary string `json:"error,omitempty"`
}
