// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.ui

import android.content.Context
import androidx.annotation.StringRes
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreContractException
import es.dipgra.grxfirma.android.core.CoreReadiness
import es.dipgra.grxfirma.android.core.CoreUnavailableException
import es.dipgra.grxfirma.android.files.InvalidDocumentException
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.SelectedFile

sealed interface UiText {
    data class Resource(@param:StringRes val id: Int, val arguments: List<Any> = emptyList()) : UiText
    data class Verification(
        val format: String,
        val signerCount: Int,
        val integrityStatus: String,
        val certificateStatus: String,
        val trustStatus: String,
        val revocationMode: String,
        val warningCount: Int,
        val errorCount: Int,
    ) : UiText
}

fun UiText.resolve(context: Context): String = when (this) {
    is UiText.Resource -> context.getString(id, *arguments.toTypedArray())
    is UiText.Verification -> buildList {
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
) {
    val canImportCertificate: Boolean
        get() = backend.available && certificateFile != null && !busy && !awaitingSave
    val canSign: Boolean
        get() = backend.available && document != null && certificate != null && !busy && !awaitingSave
    val canVerify: Boolean
        get() = backend.available && document != null && !busy && !awaitingSave
    val canForgetCertificate: Boolean
        get() = backend.available && certificate != null && !busy && !awaitingSave
    val canRetryPendingOutput: Boolean get() = awaitingSave && !busy
    val canDiscardPendingOutput: Boolean get() = awaitingSave && !busy
    val canReplaceSelection: Boolean get() = !busy && !awaitingSave
    val canAcceptIncomingDocument: Boolean get() = !busy && !awaitingSave
}

sealed interface UiEffect {
    data class SaveSignedDocument(val displayName: String, val mimeType: String) : UiEffect
}

fun Throwable.toUserText(): UiText {
    val resource = when (this) {
        is InvalidDocumentException, is SecurityException -> R.string.error_document_access
        is CoreUnavailableException -> R.string.error_core_unavailable
        is CoreContractException -> when (message) {
            CORE_PKCS12_DECODE_MESSAGE -> R.string.error_pkcs12_password_or_legacy
            CORE_CERTIFICATE_NOT_CURRENT_MESSAGE -> R.string.error_certificate_not_current
            CORE_SIGNING_IDENTITY_UNSUPPORTED_MESSAGE -> R.string.error_signing_identity_unsupported
            else -> R.string.error_core_operation
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
