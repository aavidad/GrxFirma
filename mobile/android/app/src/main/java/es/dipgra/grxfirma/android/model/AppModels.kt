// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.model

import android.net.Uri

data class SelectedFile(
    val uri: Uri,
    val displayName: String,
    val mimeType: String,
    val sizeBytes: Long?,
)

data class LoadedFile(
    val displayName: String,
    val mimeType: String,
    val bytes: ByteArray,
)

data class CertificateSummary(
    val id: String,
    val subject: String,
    val issuer: String,
    val fingerprint: String,
)

data class SignedOutput(
    val bytes: ByteArray,
    val displayName: String,
    val mimeType: String,
    val format: String,
    val algorithm: String,
)

data class VerificationSummary(
    val valid: Boolean,
    val reason: String,
    val details: List<String>,
    val signers: List<String>,
    val format: String,
    val coverage: String,
    val integrityStatus: String,
    val certificateStatus: String,
    val trustStatus: String,
    val revocationMode: String,
    val warnings: List<String>,
    val errors: List<String>,
    val signerSummaries: List<SignerSummary> = emptyList(),
    val reportJson: String = "",
)

data class SignerSummary(val id: String, val subject: String, val issuer: String, val fingerprint: String)

data class SignatureInspection(val hasSignature: Boolean, val format: String = "")

data class HashOutput(
    val algorithm: String,
    val format: String,
    val hash: String,
    val bytes: ByteArray,
    val extension: String,
)

data class HashCheck(
    val valid: Boolean,
    val algorithm: String,
    val expected: String,
    val actual: String,
)

/** Contenedores CMS del escritorio que Android ofrece. */
data class ProtectionRequest(
    val container: String,
    val recipients: List<ByteArray> = emptyList(),
    val includeSessionCertificate: Boolean = false,
    val sign: Boolean = false,
    val certificateId: String = "",
)

data class BatchItemResult(
    val sourceName: String,
    val output: SignedOutput?,
)

/** Incidencia con clave de localización cerrada del motor (verifactu.*, eni.*). */
data class EngineIssue(val field: String, val key: String, val level: String)

data class VeriFactuRecord(
    val file: String,
    val type: String,
    val hash: String,
    val calculatedHash: String,
    val previousHash: String,
    val signed: Boolean,
    val valid: Boolean,
    val issues: List<EngineIssue>,
)

data class VeriFactuReport(
    val valid: Boolean,
    val errors: Int,
    val warnings: Int,
    val records: List<VeriFactuRecord>,
    /** Respuesta del motor tal cual, para exportarla junto al informe traducido. */
    val reportJson: String = "",
)

/** Códigos oficiales de la NTI que ofrece el motor. */
data class EniCatalogs(
    val documentStates: List<String>,
    val documentTypes: List<String>,
    val fileStates: List<String>,
)

/** Metadatos obligatorios del documento ENI; la fecha va en RFC 3339. */
data class EniRequest(
    val organs: List<String>,
    val origin: String,
    val state: String,
    val documentType: String,
    val identifier: String = "",
    val sourceIdentifier: String = "",
    val captureDate: String = "",
    val contentFormat: String = "",
)

data class EniDocument(val bytes: ByteArray, val signatureType: String)

data class EniValidation(val valid: Boolean, val issues: List<EngineIssue>)

data class CsvLegend(val url: String, val text: String)

/** Certificado de la sesión con su caducidad; NIF y organización son datos del titular. */
data class CertificateDetail(
    val id: String,
    val subject: String,
    val issuer: String,
    val fingerprint: String,
    val nif: String,
    val organization: String,
    val kind: String,
    val keyType: String,
    val keyBits: Int,
    val notBefore: String,
    val notAfter: String,
    val daysLeft: Int,
    val status: String,
    val external: Boolean,
    val canEncrypt: Boolean,
    val hasOcsp: Boolean,
    val hasCrl: Boolean,
)

data class CertificateDetails(val expiringSoonDays: Int, val certificates: List<CertificateDetail>)

/** Resultado de OCSP/CRL: valid, revoked, inconclusive o unavailable. */
data class RevocationCheck(
    val status: String,
    val method: String,
    val checkedAt: String,
    val revokedAt: String,
    val hasOcsp: Boolean,
    val hasCrl: Boolean,
)

/** Datos del motor sin información personal. */
data class EngineDiagnostics(
    val engineVersion: String,
    val contractVersion: Int,
    val platform: String,
    val goVersion: String,
    val architecture: String,
    val engineTimeUtc: String,
    val sessionIdentity: Boolean,
)

/** Prueba de la TSA: status ok, invalid_url, unreachable, timeout, rejected o bad_response. */
data class TsaProbe(
    val status: String,
    val https: Boolean,
    val tsaTime: String,
    val localTime: String,
    val skewSeconds: Long,
    val elapsedMillis: Long,
)

data class VeriFactuQr(
    val url: String,
    val nif: String,
    val number: String,
    val date: String,
    val amount: String,
    val verifiable: Boolean,
    val test: Boolean,
)

/** Comprobación de versión: newer, current, not_comparable, no_releases o error. */
data class UpdateCheck(
    val status: String,
    val errorCode: String,
    val current: String,
    val latest: String,
    val url: String,
)
