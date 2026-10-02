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
import es.dipgra.grxfirma.android.files.ContentRepository
import es.dipgra.grxfirma.android.files.DocumentPolicy
import es.dipgra.grxfirma.android.model.SignedOutput
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainViewModel(
    private val repository: ContentRepository,
    private val core: CoreBridge,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) : ViewModel() {
    private val mutableState = MutableStateFlow(MainUiState(backend = core.readiness))
    val state: StateFlow<MainUiState> = mutableState.asStateFlow()

    private val effectChannel = Channel<UiEffect>(Channel.BUFFERED)
    val effects = effectChannel.receiveAsFlow()

    private var pendingOutput: SignedOutput? = null

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
            mutableState.value = mutableState.value.copy(document = selected, result = OperationResult.Idle)
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
                result = OperationResult.Idle,
            )
        }
    }

    fun clearOriginalDocument() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(
            originalDocument = null,
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
        launchOperation {
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
                val output = core.sign(loaded, format, certificate.id, options)
                pendingOutput?.bytes?.fill(0)
                pendingOutput = output
                mutableState.value = mutableState.value.copy(
                    awaitingSave = true,
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
                    result = OperationResult.Success(
                        UiText.Resource(R.string.verification_result_title),
                        UiText.Verification(
                            valid = verification.valid,
                            reason = verification.reason,
                            format = verification.format,
                            signerCount = verification.signers.size,
                            integrityStatus = verification.integrityStatus,
                            certificateStatus = verification.certificateStatus,
                            trustStatus = verification.trustStatus,
                            revocationMode = verification.revocationMode,
                            warningCount = verification.warnings.size,
                            errorCount = verification.errors.size,
                        ),
                    ),
                )
            } finally {
                loaded.bytes.fill(0)
                original?.bytes?.fill(0)
            }
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
        block: suspend () -> Unit,
    ) {
        if (mutableState.value.busy || (!allowAwaitingSave && mutableState.value.awaitingSave)) return
        mutableState.value = mutableState.value.copy(busy = true)
        viewModelScope.launch {
            try {
                withContext(ioDispatcher) { block() }
            } catch (error: Exception) {
                setError(error.toUserText())
            } finally {
                mutableState.value = mutableState.value.copy(busy = false)
            }
        }
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
        private val repository: ContentRepository,
        private val core: CoreBridge,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(MainViewModel::class.java))
            return MainViewModel(repository, core) as T
        }
    }
}
