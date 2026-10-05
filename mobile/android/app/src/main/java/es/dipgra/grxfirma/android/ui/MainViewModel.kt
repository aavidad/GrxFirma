// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.ui

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreBridge
import es.dipgra.grxfirma.android.core.ExternalIdentityBridge
import es.dipgra.grxfirma.android.nfc.DnieError
import es.dipgra.grxfirma.android.nfc.DnieErrors
import es.dipgra.grxfirma.android.nfc.DnieNfcSession
import es.dipgra.grxfirma.android.files.DocumentRepository
import es.dipgra.grxfirma.android.files.DocumentPolicy
import es.dipgra.grxfirma.android.model.SignedOutput
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.SignatureInspection
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.atomic.AtomicLong

class MainViewModel(
    private val repository: DocumentRepository,
    private val core: CoreBridge,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) : ViewModel() {
    private val mutableState = MutableStateFlow(MainUiState(backend = core.readiness))
    val state: StateFlow<MainUiState> = mutableState.asStateFlow()

    private val effectChannel = Channel<UiEffect>(Channel.BUFFERED)
    val effects = effectChannel.receiveAsFlow()

    private var pendingOutput: SignedOutput? = null
    private val identityEpoch = AtomicLong()
    val engineVersion: String get() = core.engineVersion

    fun selectDocument(uri: Uri) {
        if (!mutableState.value.canReplaceSelection) {
            reportBusyIncomingIntent()
            return
        }
        runInspect {
            val selected = repository.inspect(uri, "documento", "application/octet-stream")
            DocumentPolicy.requireAllowedSize(
                selected.sizeBytes,
                DocumentPolicy.MAX_DOCUMENT_BYTES,
                "El documento",
            )
            mutableState.value = mutableState.value.copy(document = selected, result = OperationResult.Idle,
                verification = null, postSignVerificationFailed = false, signatureAction = "sign", coSignSuggested = false, detectedSignatureFormat = "")
            inspectExistingSignature(selected)
        }
    }

    fun selectOriginalDocument(uri: Uri) {
        if (!mutableState.value.canReplaceSelection) {
            reportBusyIncomingIntent()
            return
        }
        runInspect {
            val selected = repository.inspect(uri, "documento-original", "application/octet-stream")
            DocumentPolicy.requireAllowedSize(
                selected.sizeBytes,
                DocumentPolicy.MAX_DOCUMENT_BYTES,
                "El documento original",
            )
            mutableState.value = mutableState.value.copy(
                originalDocument = selected,
                verification = null,
                postSignVerificationFailed = false,
                result = OperationResult.Idle,
            )
        }
    }

    fun clearOriginalDocument() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(
            originalDocument = null,
            verification = null,
            postSignVerificationFailed = false,
            result = OperationResult.Idle,
        )
    }

    fun selectCertificateFile(uri: Uri) {
        if (!mutableState.value.canReplaceSelection) {
            reportBusyIncomingIntent()
            return
        }
        runInspect {
            val selected = repository.inspect(uri, "certificado.p12", "application/x-pkcs12")
            DocumentPolicy.requireAllowedSize(
                selected.sizeBytes,
                DocumentPolicy.MAX_CERTIFICATE_BYTES,
                "El certificado",
            )
            mutableState.value = mutableState.value.copy(
                certificateFile = selected,
                result = OperationResult.Idle,
            )
        }
    }

    fun reportSharedIntentError(multiple: Boolean = false) {
        mutableState.value = mutableState.value.copy(
            result = OperationResult.Error(
                UiText.Resource(
                    if (multiple) R.string.error_multiple_documents else R.string.error_invalid_shared_intent,
                ),
            ),
        )
    }

    fun reportPickerError() {
        setError(UiText.Resource(R.string.error_picker_unavailable))
    }

    fun reportSavePickerUnavailable() {
        if (mutableState.value.awaitingSave) {
            setError(UiText.Resource(R.string.error_save_picker_unavailable))
        }
    }

    fun reportSavePickerCancelled() {
        if (mutableState.value.awaitingSave) {
            setError(UiText.Resource(R.string.error_save_picker_cancelled))
        }
    }

    fun reportBusyIncomingIntent() {
        mutableState.value = mutableState.value.copy(
            result = OperationResult.Error(UiText.Resource(R.string.error_operation_in_progress)),
        )
    }

    fun canAcceptIncomingDocument(): Boolean = mutableState.value.canAcceptIncomingDocument

    suspend fun previewSeal(options: Map<String, String>): ByteArray {
        val certificate = mutableState.value.certificate
            ?: throw IllegalStateException("No hay certificado para el sello.")
        return withContext(ioDispatcher) { core.sealPreview(certificate.id, options) }
    }

    fun importCertificate(password: CharArray) {
        if (!mutableState.value.canReplaceSelection) {
            password.fill('\u0000')
            return
        }
        val selected = mutableState.value.certificateFile
        if (selected == null) {
            password.fill('\u0000')
            setError(UiText.Resource(R.string.no_certificate_file))
            return
        }
        if (password.isEmpty()) {
            password.fill('\u0000')
            setError(UiText.Resource(R.string.error_password_required))
            return
        }
        launchOperation(onFinished = { password.fill('\u0000') }) {
            try {
                val loaded = repository.loadCertificate(selected)
                try {
                    val certificate = core.importCertificate(loaded.bytes, password)
                    mutableState.value = mutableState.value.copy(
                        certificate = certificate,
                        certificateFile = null,
                        result = OperationResult.Success(
                            UiText.Resource(R.string.result_success),
                            UiText.Resource(R.string.result_certificate_imported, listOf(certificate.subject)),
                        ),
                    )
                } finally {
                    loaded.bytes.fill(0)
                }
            } finally {
                password.fill('\u0000')
            }
        }
    }

    internal fun installDnie(session: DnieNfcSession, onReady: () -> Unit) {
        val epoch = identityEpoch.incrementAndGet()
        launchOperation {
            try {
                val bridge = core as? ExternalIdentityBridge
                    ?: throw IllegalStateException("EXTERNAL_SIGNER_UNAVAILABLE")
                val certificate = bridge.installExternalIdentity(
                    session.certificate.encoded,
                    session.chain.map { it.encoded },
                    session::signDigest,
                )
                withContext(Dispatchers.Main) {
                    if (identityEpoch.get() != epoch) {
                        core.clearSession()
                        session.close()
                    } else {
                        mutableState.value = mutableState.value.copy(
                            certificate = certificate,
                            certificateFile = null,
                            result = OperationResult.Success(
                                UiText.Resource(R.string.result_success),
                                UiText.Resource(R.string.dnie_ready),
                            ),
                        )
                        onReady()
                    }
                }
            } catch (error: Exception) {
                session.close()
                reportDnieError(error)
            }
        }
    }

    fun clearDnieIdentity() {
        identityEpoch.incrementAndGet()
        try { core.clearSession() } catch (_: Exception) { }
        val current = mutableState.value
        mutableState.value = current.copy(
            certificate = null,
            result = if (current.awaitingSave) current.result else
                OperationResult.Error(UiText.Resource(R.string.dnie_session_ended)),
        )
    }

    fun reportDnieError(error: Throwable, retriesLeft: Int = -1) {
        val resource = when (DnieErrors.from(error, retriesLeft)) {
            DnieError.NFC_MISSING -> R.string.dnie_nfc_missing
            DnieError.NFC_OFF -> R.string.dnie_nfc_off
            DnieError.CAN -> R.string.dnie_can_wrong
            DnieError.PIN -> if (retriesLeft >= 0) R.string.dnie_pin_wrong else R.string.dnie_pin_wrong_unknown
            DnieError.BLOCKED -> R.string.dnie_blocked
            DnieError.REMOVED -> R.string.dnie_removed
            DnieError.EXPIRED -> R.string.dnie_expired
            DnieError.CERTIFICATE -> R.string.dnie_no_signature_certificate
            DnieError.LIBRARY -> R.string.dnie_library_unavailable
            DnieError.OTHER -> R.string.dnie_error
        }
        val args = if (resource == R.string.dnie_pin_wrong) listOf(retriesLeft.coerceAtLeast(0)) else emptyList()
        setError(UiText.Resource(resource, args))
    }

    fun sign(format: String, options: Map<String, String> = emptyMap()) {
        val snapshot = mutableState.value
        val document = snapshot.document
        val certificate = snapshot.certificate
        if (document == null) {
            setError(UiText.Resource(R.string.error_document_required))
            return
        }
        if (certificate == null) {
            setError(UiText.Resource(R.string.error_certificate_required))
            return
        }
        launchOperation {
            val loaded = repository.loadDocument(document)
            try {
                val configured = try {
                    SigningOptions.create(snapshot.signatureProfile, snapshot.tsaEnabled, snapshot.tsaUrl)
                } catch (_: Exception) {
                    setError(UiText.Resource(R.string.error_tsa_configuration))
                    return@launchOperation
                }
                val effectiveFormat = if (format == "auto") when {
                    snapshot.signatureAction != "sign" && snapshot.detectedSignatureFormat.isNotBlank() -> snapshot.detectedSignatureFormat
                    document.mimeType == "application/pdf" || document.displayName.endsWith(".pdf", true) -> "pades"
                    document.mimeType.contains("xml") || document.displayName.endsWith(".xml", true) -> "xades"
                    else -> "cades"
                } else format
                if (!SigningOptions.supported(effectiveFormat, snapshot.signatureAction, snapshot.signatureProfile)) {
                    setError(UiText.Resource(R.string.error_signature_combination))
                    return@launchOperation
                }
                val output = core.sign(loaded, effectiveFormat, certificate.id, options + configured, snapshot.signatureAction)
                pendingOutput?.bytes?.fill(0)
                pendingOutput = output
                // The output remains saveable even if verification cannot run.
                // Detached signatures need the input for sign, and the separately
                // selected original for co/countersign (never the old signature).
                var original: LoadedFile? = null
                val verification = try {
                    original = if (!output.format.equals("cades", true)) null else
                        if (snapshot.signatureAction == "sign") loaded else snapshot.originalDocument?.let(repository::loadDocument)
                    core.verify(LoadedFile(output.displayName, output.mimeType, output.bytes), original)
                } catch (_: Exception) {
                    null
                } finally {
                    if (original !== loaded) original?.bytes?.fill(0)
                }
                mutableState.value = mutableState.value.copy(
                    awaitingSave = true,
                    verification = verification,
                    verifiedDocumentName = output.displayName,
                    postSignVerificationFailed = verification == null,
                    result = OperationResult.Success(
                        UiText.Resource(R.string.result_success),
                        UiText.Resource(
                            R.string.result_signed,
                            listOf(output.format, output.algorithm),
                        ),
                    ),
                )
                effectChannel.send(UiEffect.SaveSignedDocument(output.displayName, output.mimeType))
            } finally {
                loaded.bytes.fill(0)
            }
        }
    }

    fun verify() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(verification = null, postSignVerificationFailed = false)
        val snapshot = mutableState.value
        val document = snapshot.document
        if (document == null) {
            setError(UiText.Resource(R.string.error_document_required))
            return
        }
        launchOperation {
            val loaded = repository.loadDocument(document)
            var original: es.dipgra.grxfirma.android.model.LoadedFile? = null
            try {
                original = snapshot.originalDocument?.let(repository::loadDocument)
                val verification = core.verify(loaded, original)
                mutableState.value = mutableState.value.copy(
                    verification = verification,
                    verifiedDocumentName = document.displayName,
                    postSignVerificationFailed = false,
                    coSignSuggested = verification.signers.isNotEmpty() || verification.signerSummaries.isNotEmpty(),
                    detectedSignatureFormat = verification.format.lowercase().takeIf { it in listOf("cades", "pades", "xades") }.orEmpty(),
                    result = OperationResult.Success(
                        UiText.Resource(R.string.verification_result_title),
                        verification.toUiText(),
                    ),
                )
            } finally {
                loaded.bytes.fill(0)
                original?.bytes?.fill(0)
            }
        }
    }

    fun updateSigningSettings(action: String, profile: String, tsaEnabled: Boolean, tsaUrl: String) {
        if (!mutableState.value.canReplaceSelection) return
        require(action in listOf("sign", "cosign", "countersign"))
        require(profile in listOf("baseline", "t", "lt", "lta"))
        mutableState.value = mutableState.value.copy(signatureAction = action, signatureProfile = profile,
            tsaEnabled = tsaEnabled, tsaUrl = tsaUrl.take(2048))
    }

    fun acceptCoSignSuggestion() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(signatureAction = "cosign", coSignSuggested = false)
    }

    private fun inspectExistingSignature(document: es.dipgra.grxfirma.android.model.SelectedFile) {
        if (!core.readiness.available) return
        launchOperation {
            val loaded = repository.loadDocument(document)
            try {
                val inspection = try { core.inspectSignature(loaded) } catch (_: Exception) { SignatureInspection(false) }
                mutableState.value = mutableState.value.copy(coSignSuggested = inspection.hasSignature,
                    detectedSignatureFormat = inspection.format)
            } finally { loaded.bytes.fill(0) }
        }
    }

    fun exportVerificationReport() {
        if (!mutableState.value.canExportReport) return
        mutableState.value = mutableState.value.copy(awaitingReportSave = true)
        viewModelScope.launch { effectChannel.send(UiEffect.SaveVerificationReport) }
    }

    fun cancelReportExport() {
        if (!mutableState.value.awaitingReportSave) return
        mutableState.value = mutableState.value.copy(awaitingReportSave = false,
            result = OperationResult.Error(UiText.Resource(R.string.error_report_export)))
    }

    fun saveVerificationReport(uri: Uri) {
        if (!mutableState.value.awaitingReportSave) return
        val report = mutableState.value.verification?.reportJson ?: return cancelReportExport()
        mutableState.value = mutableState.value.copy(awaitingReportSave = false)
        launchOperation {
            val bytes = report.encodeToByteArray()
            try {
                repository.write(uri, bytes)
                mutableState.value = mutableState.value.copy(
                    result = OperationResult.Success(UiText.Resource(R.string.result_report_saved)))
            } catch (_: Exception) {
                setError(UiText.Resource(R.string.error_report_export))
            } finally { bytes.fill(0) }
        }
    }

    fun savePendingOutput(uri: Uri) = launchOperation(allowAwaitingSave = true) {
        val output = pendingOutput
            ?: throw IllegalStateException("No hay un resultado de firma pendiente.")
        try {
            repository.write(uri, output.bytes)
            output.bytes.fill(0)
            pendingOutput = null
            mutableState.value = mutableState.value.copy(
                awaitingSave = false,
                result = OperationResult.Success(UiText.Resource(R.string.result_saved)),
            )
        } catch (_: Exception) {
            // Un proveedor SAF puede haber creado un fichero parcial. La salida se
            // conserva solo en memoria para poder elegir otro destino sin refirmar.
            mutableState.value = mutableState.value.copy(
                result = OperationResult.Error(UiText.Resource(R.string.error_save_failed_keep_output)),
            )
        }
    }

    fun retryPendingOutput() = launchOperation(allowAwaitingSave = true) {
        val output = pendingOutput
            ?: throw IllegalStateException("No hay un resultado de firma pendiente.")
        effectChannel.send(UiEffect.SaveSignedDocument(output.displayName, output.mimeType))
    }

    fun discardPendingOutput() {
        if (!mutableState.value.canDiscardPendingOutput) return
        pendingOutput?.bytes?.fill(0)
        pendingOutput = null
        val result = try {
            core.clearSession()
            OperationResult.Error(UiText.Resource(R.string.result_save_cancelled))
        } catch (error: Exception) {
            OperationResult.Error(error.toUserText())
        }
        mutableState.value = mutableState.value.copy(
            awaitingSave = false,
            certificate = null,
            result = result,
        )
    }

    fun forgetCertificate() {
        if (!mutableState.value.canForgetCertificate) return
        try {
            core.clearSession()
            mutableState.value = mutableState.value.copy(
                certificate = null,
                certificateFile = null,
                result = OperationResult.Success(UiText.Resource(R.string.result_certificate_forgotten)),
            )
        } catch (error: Exception) {
            setError(error.toUserText())
        }
    }

    private fun runInspect(block: () -> Unit) {
        try {
            block()
        } catch (error: Exception) {
            setError(error.toUserText())
        }
    }

    private fun launchOperation(
        allowAwaitingSave: Boolean = false,
        onFinished: () -> Unit = {},
        block: suspend () -> Unit,
    ) {
        if (mutableState.value.busy || mutableState.value.awaitingReportSave || (!allowAwaitingSave && mutableState.value.awaitingSave)) {
            onFinished()
            return
        }
        mutableState.value = mutableState.value.copy(busy = true)
        viewModelScope.launch {
            try {
                withContext(ioDispatcher) { block() }
            } catch (error: Exception) {
                setError(error.toUserText())
            } finally {
                mutableState.value = mutableState.value.copy(busy = false)
            }
        }.invokeOnCompletion { onFinished() }
    }

    private fun setError(text: UiText) {
        mutableState.value = mutableState.value.copy(result = OperationResult.Error(text))
    }

    override fun onCleared() {
        pendingOutput?.bytes?.fill(0)
        pendingOutput = null
        try {
            core.clearSession()
        } catch (_: Exception) {
            // El cierre de la UI no debe conservar una referencia a material sensible.
        }
        super.onCleared()
    }

    class Factory(
        private val repository: DocumentRepository,
        private val core: CoreBridge,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(MainViewModel::class.java))
            return MainViewModel(repository, core) as T
        }
    }
}
