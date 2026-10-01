// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package application contiene los casos de uso de GrxFirma y los DTOs internos
// que usan para comunicarse con los adaptadores de entrada y salida.
//
// Regla fundamental: ningun tipo de este paquete puede ser importado o reutilizado
// directamente por un adaptador como DTO de integracion. Los adaptadores deben
// traducir sus estructuras propias a estos tipos en la capa anticorrupcion.
package application

import "grxfirma/internal/domain"

// --- Comandos de firma ---

// SignCommand representa la solicitud de firma de un unico documento.
type SignCommand struct {
	// Documento a firmar.
	Document domain.Document

	// Formato de firma deseado (CAdES, XAdES, PAdES).
	Format domain.SignatureFormat

	// Accion de firma (sign, cosign, countersign).
	Action domain.SignatureAction

	// CertificateID es el identificador del certificado a usar.
	// Si esta vacio, el caso de uso solicitara seleccion al usuario.
	CertificateID string

	// Options contiene parametros adicionales especificos del formato.
	// Las claves son nombres de dominio neutros, nunca nombres de parametros de protocolo.
	Options map[string]string

	// RequesterApplication identifica la aplicación o canal que solicita la
	// operación. Los adaptadores deben obtenerla de un canal autenticado y no
	// de un campo controlado por el documento o por una página web.
	RequesterApplication string

	// RequesterOrigin conserva el origen visible de la solicitud cuando el
	// borde de entrada puede acreditarlo (por ejemplo, una WebExtension).
	RequesterOrigin string
}

// CoSignCommand representa la solicitud de cofirma sobre un documento ya firmado.
type CoSignCommand struct {
	// SignedDocument es el documento que ya contiene una firma previa.
	SignedDocument domain.Document

	Format        domain.SignatureFormat
	CertificateID string
	Options       map[string]string
}

// CounterSignCommand representa la solicitud de contrafirma sobre una firma existente.
type CounterSignCommand struct {
	// SignedDocument es el documento que contiene la firma a contrafirmar.
	SignedDocument domain.Document

	Format        domain.SignatureFormat
	CertificateID string
	Options       map[string]string
}

// MultiCoSignCommand representa la solicitud guiada de firma/cofirma secuencial
// de un único documento con varios certificados, generando una salida única.
type MultiCoSignCommand struct {
	// Document es el documento base a firmar o cofirmar.
	Document domain.Document

	// Format fija el formato de firma de toda la secuencia.
	Format domain.SignatureFormat

	// InitialAction define la primera operación:
	// sign para documento no firmado, cosign para documento ya firmado.
	InitialAction domain.SignatureAction

	// PrimaryCertificateID es el certificado de la primera operación.
	PrimaryCertificateID string

	// AdditionalCertificateIDs contiene los certificados adicionales que
	// se aplicarán secuencialmente como cofirma.
	AdditionalCertificateIDs []string

	// Options contiene parámetros neutros de dominio para la primera firma.
	Options map[string]string
}

// --- Comandos de seleccion de certificado ---

// SelectCertificateCommand representa la solicitud de seleccion de certificado.
type SelectCertificateCommand struct {
	// Filtros opcionales para limitar los certificados mostrados al usuario.
	SubjectFilter string
	IssuerFilter  string

	// SoloNoCaducados indica si se deben excluir los certificados expirados.
	SoloNoCaducados bool
}

// --- Comandos de lote ---

// ProcessBatchCommand representa la solicitud de procesado de un lote de firmas.
type ProcessBatchCommand struct {
	// Jobs es la lista de trabajos de firma individuales del lote.
	Jobs []domain.SignatureJob

	// StopOnError detiene el lote tras el primer trabajo fallido. Los adaptadores
	// que necesiten representar los trabajos omitidos conservan el orden de Jobs.
	StopOnError bool

	// CertificateID fija el certificado a usar para todo el lote.
	// Si está vacío, el caso de uso aplicará la misma resolución que en firma simple.
	CertificateID string

	// Session es la sesion de intercambio remoto para subir y recuperar resultados.
	Session domain.ExchangeSession
}

// UploadResultCommand representa la solicitud de subida de un resultado al servidor remoto.
type UploadResultCommand struct {
	Session domain.ExchangeSession
	Data    []byte
}

// RetrieveRequestCommand representa la solicitud de descarga de una peticion del servidor remoto.
type RetrieveRequestCommand struct {
	Session domain.ExchangeSession
}

// --- Comandos de verificacion ---

// VerifyCommand representa la solicitud de verificacion de un documento firmado.
type VerifyCommand struct {
	// SignedDocument es el documento firmado a verificar.
	SignedDocument domain.Document

	// OriginalDocument contiene el contenido original cuando la firma es detached
	// y la verificación no puede resolverse solo con el documento firmado.
	OriginalDocument *domain.Document
}

// --- Comandos de proteccion de ficheros ---

// ProtectCommand representa la solicitud de proteger/cifrar un documento para uno
// o varios destinatarios.
type ProtectCommand struct {
	Document domain.Document

	// Profile fija el perfil criptográfico a usar.
	Profile domain.ProtectionProfile

	// RecipientIDs identifica los destinatarios de la envolvente.
	RecipientIDs []string

	// Options queda reservado para parámetros neutros de dominio.
	Options map[string]string

	// SymmetricKey contiene una clave AES transitoria ya decodificada. El
	// adaptador de entrada conserva su propiedad y la borra tras Execute.
	SymmetricKey []byte
}

// ProtectAndSignCommand representa la solicitud de proteger/cifrar y firmar un
// documento en una sola operación, manteniéndolo como caso de uso separado.
type ProtectAndSignCommand struct {
	Document domain.Document
	Profile  domain.ProtectionProfile

	// RecipientIDs identifica los destinatarios del contenedor protegido.
	RecipientIDs []string

	// CertificateID fija el certificado del remitente firmante.
	CertificateID string

	// Options contiene parámetros neutros específicos del contenedor.
	Options map[string]string
}

// UnprotectCommand representa la solicitud de descifrar un documento protegido.
type UnprotectCommand struct {
	ProtectedDocument domain.Document

	// Options queda reservado para parámetros neutros de dominio o material
	// de compatibilidad de clientes anteriores.
	Options map[string]string

	// SymmetricKey contiene una clave AES transitoria ya decodificada. El
	// adaptador de entrada conserva su propiedad y la borra tras Execute.
	SymmetricKey []byte
}

// ExportProtectionRecipientCommand representa la exportación del material
// público de un destinatario de protección.
type ExportProtectionRecipientCommand struct {
	RecipientID string
}

// ImportProtectionRecipientCommand representa la importación de un destinatario
// público de protección externo.
type ImportProtectionRecipientCommand struct {
	Data []byte
}

// --- Comandos de confianza ---

// TrustAction representa la accion a realizar sobre un dominio de confianza.
type TrustAction string

const (
	// TrustActionAllow anade el dominio a la lista de permitidos.
	TrustActionAllow TrustAction = "allow"

	// TrustActionDeny deniega el dominio explicitamente.
	TrustActionDeny TrustAction = "deny"

	// TrustActionRemove elimina el dominio de la lista de confianza.
	TrustActionRemove TrustAction = "remove"
)

// ManageTrustedDomainCommand representa la solicitud de gestion de la lista de dominios de confianza.
type ManageTrustedDomainCommand struct {
	Origin string
	Action TrustAction
}

// --- Comandos de importacion ---

// ImportCertificateCommand representa la solicitud de importacion de un certificado externo.
type ImportCertificateCommand struct {
	// Data son los bytes del fichero de certificado (P12, DER, PEM, etc.).
	Data []byte

	// Password es la contrasena de proteccion del fichero, si aplica.
	Password string
}

type ListCertificateAccessOptionsCommand struct{}

type OpenCertificateManagerCommand struct {
	ManagerID string
}

type ImportCertificateToStoreCommand struct {
	TargetID string
	Data     []byte
	Password string
}

type UseTemporaryCertificateCommand struct {
	Data     []byte
	Password string
}

type RemoveTemporaryCertificateCommand struct {
	CertificateID string
}

// ResolvePlatformProfileCommand representa la solicitud de resolucion
// de capacidades observables de la plataforma actual.
type ResolvePlatformProfileCommand struct{}

// NotifyUserCommand representa la solicitud de mostrar una notificacion
// al usuario por el canal disponible en la plataforma actual.
type NotifyUserCommand struct {
	Title string
	Body  string
}

// --- Comandos de auditoria ---

// AuditCommand representa una operacion que debe quedar registrada en el sistema de evidencias.
type AuditCommand struct {
	// OperationType describe el tipo de operacion realizada.
	OperationType string

	// Origin es el origen de la solicitud (dominio, adaptador, etc.).
	Origin string

	// CertificateFingerprint es la huella SHA-256 del certificado usado, si aplica.
	CertificateFingerprint string

	// CertificateID es el identificador del certificado usado, si aplica.
	// Se mantiene por compatibilidad; si existe CertificateFingerprint, este tiene prioridad.
	CertificateID string

	// DocumentName es el nombre del documento afectado, si aplica.
	DocumentName string

	// DocumentData contiene los bytes del documento o payload relevante solo para calcular su hash.
	// Nunca se persiste directamente.
	DocumentData []byte

	// DocumentHash permite inyectar un hash ya calculado si no se quiere pasar DocumentData.
	DocumentHash string

	// Format describe el formato funcional de la operacion, si aplica.
	Format string

	// Success indica si la operacion termino con exito.
	Success bool

	// ErrorSummary es un resumen sanitizado del error, si hubo alguno.
	// Nunca debe contener payloads, valor-pruebas ni material criptografico.
	ErrorSummary string
}
