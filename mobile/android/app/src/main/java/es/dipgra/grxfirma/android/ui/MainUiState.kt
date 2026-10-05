// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.ui

import android.content.Context
import androidx.annotation.PluralsRes
import androidx.annotation.StringRes
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreContractException
import es.dipgra.grxfirma.android.core.CoreReadiness
import es.dipgra.grxfirma.android.core.CoreUnavailableException
import es.dipgra.grxfirma.android.core.DocumentServices
import es.dipgra.grxfirma.android.core.SignatureFormats
import es.dipgra.grxfirma.android.files.InvalidDocumentException
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.model.VerificationSummary
import es.dipgra.grxfirma.android.model.SignerSummary

sealed interface UiText {
    data class Resource(@param:StringRes val id: Int, val arguments: List<Any> = emptyList()) : UiText
    data class Plural(@param:PluralsRes val id: Int, val count: Int) : UiText
    data class Lines(val lines: List<UiText>) : UiText
    /** Clave cerrada del catálogo del motor (verifactu.*, eni.*, csv.error.*). */
    data class Engine(val key: String) : UiText
    data class Verification(
        val valid: Boolean,
        val reason: String = "",
        val format: String,
        val signerCount: Int,
        val integrityStatus: String,
        val certificateStatus: String,
        val trustStatus: String,
        val revocationMode: String,
        val warningCount: Int,
        val errorCount: Int,
        val coverage: String = "",
        val signers: List<String> = emptyList(),
        val signerSummaries: List<SignerSummary> = emptyList(),
        val details: List<String> = emptyList(),
        val warnings: List<String> = emptyList(),
        val errors: List<String> = emptyList(),
    ) : UiText {
        fun accredited(): Boolean = valid &&
            integrityStatus == "valid" &&
            certificateStatus == "valid" &&
            trustStatus == "valid"
    }
}

fun UiText.resolve(context: Context): String = when (this) {
    is UiText.Resource -> context.getString(id, *arguments.map { if (it is UiText) it.resolve(context) else it }.toTypedArray())
    is UiText.Plural -> context.resources.getQuantityString(id, count, count)
    is UiText.Lines -> lines.joinToString("\n") { it.resolve(context) }
    is UiText.Engine -> if (EngineKeys.isClosed(key)) EngineText.resolve(context, key) else context.getString(R.string.error_core_operation)
    is UiText.Verification -> buildList {
        add(
            context.getString(
                when {
                    reason == "revocación no concluyente" -> R.string.verification_revocation_inconclusive
                    !valid || integrityStatus == "invalid" -> R.string.verification_result_invalid
                    accredited() -> R.string.verification_result_accredited
                    else -> R.string.verification_result_incomplete
                },
            ),
        )
        add(
            context.getString(
                R.string.verification_integrity,
                context.verificationStatusLabel(integrityStatus),
            ),
        )
        add(
            context.getString(
                R.string.verification_certificate,
                context.verificationStatusLabel(certificateStatus),
            ),
        )
        add(
            context.getString(
                R.string.verification_trust,
                context.verificationStatusLabel(trustStatus),
            ),
        )
        add(
            context.getString(
                R.string.verification_revocation,
                context.revocationModeLabel(revocationMode),
            ),
        )
        if (format.isNotBlank()) add(context.getString(R.string.verification_format, format))
        add(context.getString(R.string.verification_coverage, context.getString(when (coverage) {
            "full", "total", "whole_document" -> R.string.coverage_full
            "partial", "partial_document" -> R.string.coverage_partial
            "detached" -> R.string.coverage_detached
            else -> R.string.verification_status_unknown
        })))
        if (reason.isNotBlank()) add(context.getString(R.string.verification_verdict, EngineText.resolve(context, reason)))
        if (signerSummaries.isNotEmpty()) {
            signerSummaries.forEach { signer ->
                add(context.getString(R.string.verification_signer_detail,
                    signer.subject.ifBlank { signer.id }, signer.issuer, signer.fingerprint))
            }
        } else signers.forEach { add(context.getString(R.string.verification_signer, it)) }
        details.forEach { add(context.getString(R.string.verification_evidence, EngineText.resolve(context, it))) }
        warnings.forEach { add(context.getString(R.string.verification_warning, EngineText.resolve(context, it))) }
        errors.forEach { add(context.getString(R.string.verification_error, EngineText.resolve(context, it))) }
        add(context.resources.getQuantityString(R.plurals.verification_signers, signerCount, signerCount))
        if (warningCount > 0) {
            add(
                context.resources.getQuantityString(
                    R.plurals.verification_warnings,
                    warningCount,
                    warningCount,
                ),
            )
        }
        if (errorCount > 0) {
            add(
                context.resources.getQuantityString(
                    R.plurals.verification_errors,
                    errorCount,
                    errorCount,
                ),
            )
        }
    }.joinToString("\n")
}

private fun Context.verificationStatusLabel(status: String): String = getString(
    when (status) {
        "valid" -> R.string.verification_status_valid
        "invalid" -> R.string.verification_status_invalid
        "warning" -> R.string.verification_status_warning
        else -> R.string.verification_status_unknown
    },
)

private fun Context.revocationModeLabel(mode: String): String = getString(
    when (mode) {
        "embedded_evidence_only" -> R.string.revocation_embedded_only
        "online" -> R.string.revocation_online
        else -> R.string.revocation_not_available
    },
)

sealed interface OperationResult {
    data object Idle : OperationResult
    data class Success(val title: UiText, val detail: UiText? = null) : OperationResult
    data class Error(val detail: UiText) : OperationResult
}

data class MainUiState(
    val backend: CoreReadiness,
    val document: SelectedFile? = null,
    val originalDocument: SelectedFile? = null,
    val certificateFile: SelectedFile? = null,
    val certificate: CertificateSummary? = null,
    val busy: Boolean = false,
    val awaitingSave: Boolean = false,
    val result: OperationResult = OperationResult.Idle,
    val verification: VerificationSummary? = null,
    val verifiedDocumentName: String = "",
    val postSignVerificationFailed: Boolean = false,
    val signatureAction: String = "sign",
    val signatureProfile: String = "baseline",
    val tsaEnabled: Boolean = false,
    val tsaUrl: String = "",
    val coSignSuggested: Boolean = false,
    val detectedSignatureFormat: String = "",
    val awaitingReportSave: Boolean = false,
    val toolsAvailable: Boolean = false,
    val certificateExternal: Boolean = false,
    val pendingKind: PendingKind = PendingKind.SIGNATURE,
    val hashAlgorithm: String = "SHA-256",
    val hashFormat: String = "hex",
    val protectionContainer: String = "cms",
    val protectForMe: Boolean = true,
    val recipients: List<SelectedFile> = emptyList(),
    val batchDocuments: List<SelectedFile> = emptyList(),
    val signingFormats: List<String> = SignatureFormats.BASIC,
    val documentServices: Set<String> = emptySet(),
    val verifactuRecords: List<SelectedFile> = emptyList(),
    /** Fecha de captura ENI elegida en el calendario (medianoche UTC) o null para «ahora». */
    val eniCaptureDate: Long? = null,
) {
    private val idle: Boolean get() = !busy && !awaitingSave && !awaitingReportSave
    private fun offers(service: String): Boolean = backend.available && service in documentServices
    val verifactuAvailable: Boolean get() = offers(DocumentServices.VERIFACTU)
    val eniDocumentAvailable: Boolean get() = offers(DocumentServices.ENI_DOCUMENT)
    val eniValidateAvailable: Boolean get() = offers(DocumentServices.ENI_VALIDATE)
    val csvLegendAvailable: Boolean get() = offers(DocumentServices.CSV_LEGEND)
    val canCheckVeriFactu: Boolean get() = verifactuAvailable && idle && verifactuRecords.isNotEmpty()
    val canCreateEni: Boolean get() = eniDocumentAvailable && idle && document != null
    val canValidateEni: Boolean get() = eniValidateAvailable && idle
    val canUseTools: Boolean get() = backend.available && toolsAvailable && idle
    val canHash: Boolean get() = canUseTools && document != null
    val canProtect: Boolean get() = canUseTools && document != null
    val canProtectAndSign: Boolean
        get() = canProtect && certificate != null && !certificateExternal && protectionContainer != "cms-encrypted"
    val canUnprotect: Boolean get() = canUseTools && document != null
    val canSignBatch: Boolean
        get() = canUseTools && batchDocuments.isNotEmpty() && certificate != null && !certificateExternal
    val usesTransientKey: Boolean get() = protectionContainer == "cms-encrypted"
    /** El certificado de FIRMA del DNIe no sirve para cifrar; solo el PKCS#12. */
    val canProtectForMe: Boolean get() = protectForMe && certificate != null && !certificateExternal
    val canExportReport: Boolean get() = verification?.reportJson?.isNotEmpty() == true && !busy && !awaitingSave && !awaitingReportSave
    val canImportCertificate: Boolean
        get() = backend.available && certificateFile != null && !busy && !awaitingSave && !awaitingReportSave
    val canSign: Boolean
        get() = backend.available && document != null && certificate != null && !busy && !awaitingSave && !awaitingReportSave
    val canVerify: Boolean
        get() = backend.available && document != null && !busy && !awaitingSave && !awaitingReportSave
    val canForgetCertificate: Boolean
        get() = backend.available && certificate != null && !busy && !awaitingSave && !awaitingReportSave
    val canRetryPendingOutput: Boolean get() = awaitingSave && !busy
    val canDiscardPendingOutput: Boolean get() = awaitingSave && !busy
    val canReplaceSelection: Boolean get() = !busy && !awaitingSave && !awaitingReportSave
    val canAcceptIncomingDocument: Boolean get() = !busy && !awaitingSave && !awaitingReportSave
}

/** Qué espera guardarse: una firma, un fichero de una herramienta o el lote. */
enum class PendingKind { SIGNATURE, TOOL, BATCH }

sealed interface UiEffect {
    data class SaveSignedDocument(val displayName: String, val mimeType: String) : UiEffect
    data object SaveVerificationReport : UiEffect
    data object ChooseBatchFolder : UiEffect
}

fun Throwable.toUserText(): UiText {
    val resource = when (this) {
        is InvalidDocumentException, is SecurityException -> R.string.error_document_access
        is CoreUnavailableException -> R.string.error_core_unavailable
        is CoreContractException -> when (message) {
            CORE_PKCS12_DECODE_MESSAGE -> R.string.error_pkcs12_password_or_legacy
            CORE_CERTIFICATE_NOT_CURRENT_MESSAGE -> R.string.error_certificate_not_current
            CORE_SIGNING_IDENTITY_UNSUPPORTED_MESSAGE -> R.string.error_signing_identity_unsupported
            CORE_HASH_FILE_INVALID_MESSAGE -> R.string.error_hash_file
            CORE_PROTECTION_KEY_INVALID_MESSAGE -> R.string.error_protect_key
            CORE_PROTECTION_RECIPIENT_MESSAGE -> R.string.error_protect_recipient
            CORE_UNPROTECT_FAILED_MESSAGE -> R.string.error_unprotect_failed
            CORE_PROTECT_SIGN_IDENTITY_MESSAGE -> R.string.error_protect_sign_identity
            CORE_FORMAT_REQUIRES_RSA_MESSAGE -> R.string.error_format_requires_rsa
            else -> {
                val key = message.orEmpty()
                EngineKeys.localResource(key)?.let { return UiText.Resource(it) }
                if (EngineKeys.isClosed(key)) return UiText.Engine(key)
                R.string.error_core_operation
            }
        }
        else -> R.string.error_operation_failed
    }
    return UiText.Resource(resource)
}

// Solo se localizan mensajes cerrados emitidos por la fachada. Cualquier
// detalle inesperado del núcleo conserva el error genérico y no llega a la UI.
private const val CORE_PKCS12_DECODE_MESSAGE =
    "No se pudo abrir el PKCS#12. Compruebe la contraseña; si usa un formato antiguo, reexpórtelo como PKCS#12 moderno (AES y SHA-256)."
private const val CORE_CERTIFICATE_NOT_CURRENT_MESSAGE =
    "El certificado no está vigente. Use un certificado vigente; si ha caducado, renuévelo."
private const val CORE_SIGNING_IDENTITY_UNSUPPORTED_MESSAGE =
    "El certificado o su clave no son aptos para firmar en Android. Use un certificado de firma con RSA de al menos 2048 bits o ECDSA de al menos 256 bits."

private const val CORE_HASH_FILE_INVALID_MESSAGE =
    "El fichero de huella no es válido o usa un algoritmo no admitido."
private const val CORE_PROTECTION_KEY_INVALID_MESSAGE =
    "La clave de EncryptedData debe ser AES-256 en Base64 canónico (44 caracteres)."
private const val CORE_PROTECTION_RECIPIENT_MESSAGE =
    "El certificado del destinatario no permite cifrar. Use un certificado público RSA vigente de al menos 2048 bits con cifrado de clave."
private const val CORE_UNPROTECT_FAILED_MESSAGE =
    "No se pudo desproteger el fichero. Compruebe que va dirigido a su certificado o que la clave es correcta."
private const val CORE_FORMAT_REQUIRES_RSA_MESSAGE =
    "El formato elegido solo admite certificados con clave RSA."
private const val CORE_PROTECT_SIGN_IDENTITY_MESSAGE =
    "Para proteger y firmar hace falta un certificado PKCS#12 importado en la sesión."

fun VerificationSummary.toUiText() = UiText.Verification(
    valid, reason, format, signers.size, integrityStatus, certificateStatus, trustStatus,
    revocationMode, warnings.size, errors.size, coverage, signers, signerSummaries, details, warnings, errors,
)
