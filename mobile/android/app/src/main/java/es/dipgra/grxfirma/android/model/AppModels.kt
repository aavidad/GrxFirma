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
