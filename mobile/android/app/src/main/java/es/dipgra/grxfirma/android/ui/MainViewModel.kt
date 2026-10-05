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
import es.dipgra.grxfirma.android.model.BatchItemResult
import es.dipgra.grxfirma.android.model.ProtectionRequest
import es.dipgra.grxfirma.android.model.CsvLegend
import es.dipgra.grxfirma.android.model.EniCatalogs
import es.dipgra.grxfirma.android.model.EniRequest
import es.dipgra.grxfirma.android.model.EniFileRequest
import es.dipgra.grxfirma.android.model.BatchItemInput
import es.dipgra.grxfirma.android.core.SignatureFormats
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
    private val mutableState = MutableStateFlow(
        MainUiState(
            backend = core.readiness,
            toolsAvailable = core.readiness.available && core.toolsAvailable,
            signingFormats = core.signingFormats,
            documentServices = if (core.readiness.available) core.documentServices else emptySet(),
            capabilities = if (core.readiness.available) core.capabilities else emptySet(),
        ),
    )
    val state: StateFlow<MainUiState> = mutableState.asStateFlow()

    private val effectChannel = Channel<UiEffect>(Channel.BUFFERED)
    val effects = effectChannel.receiveAsFlow()

    private var pendingOutput: SignedOutput? = null
    private var pendingBatch: List<BatchItemResult>? = null
    private var pendingSavedDetail: UiText? = null
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
            setError(UiText.Resource(if (mutableState.value.pendingKind == PendingKind.SIGNATURE)
                R.string.error_save_picker_unavailable else R.string.error_tool_save_picker_unavailable))
        }
    }

    fun reportSavePickerCancelled() {
        if (mutableState.value.awaitingSave) {
            setError(UiText.Resource(if (mutableState.value.pendingKind == PendingKind.SIGNATURE)
                R.string.error_save_picker_cancelled else R.string.error_tool_save_picker_cancelled))
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
                        certificateExternal = false,
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
                            certificateExternal = true,
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
            certificateExternal = false,
            result = if (current.awaitingSave) current.result else
                OperationResult.Error(UiText.Resource(R.string.dnie_session_ended)),
        )
    }

    fun reportDnieError(error: Throwable, retriesLeft: Int = -1) {
        setError(dnieText(error, retriesLeft))
    }

    private fun dnieText(error: Throwable, retriesLeft: Int): UiText {
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
        return UiText.Resource(resource, args)
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
                val effectiveFormat = if (format == "auto") when {
                    snapshot.signatureAction != "sign" && snapshot.detectedSignatureFormat.isNotBlank() -> snapshot.detectedSignatureFormat
                    else -> FormatPolicy.detect(document.displayName, document.mimeType, loaded.bytes)
                } else format
                if (effectiveFormat !in snapshot.signingFormats ||
                    !SigningOptions.supported(effectiveFormat, snapshot.signatureAction, snapshot.signatureProfile)) {
                    setError(UiText.Resource(R.string.error_signature_combination))
                    return@launchOperation
                }
                // Los formatos que solo generan B no reciben la TSA.
                val withTimestamp = snapshot.tsaEnabled && FormatPolicy.acceptsTimestamp(effectiveFormat)
                val configured = try {
                    SigningOptions.create(snapshot.signatureProfile, withTimestamp, snapshot.tsaUrl)
                } catch (_: Exception) {
                    setError(UiText.Resource(R.string.error_tsa_configuration))
                    return@launchOperation
                }
                val output = core.sign(loaded, effectiveFormat, certificate.id, options + configured, snapshot.signatureAction)
                replacePending(output, PendingKind.SIGNATURE)
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
                    detectedSignatureFormat = verification.format.lowercase().takeIf { it in mutableState.value.signingFormats }.orEmpty(),
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
        val signature = mutableState.value.pendingKind == PendingKind.SIGNATURE
        try {
            repository.write(uri, output.bytes)
            output.bytes.fill(0)
            pendingOutput = null
            val saved = UiText.Resource(if (signature) R.string.result_saved else R.string.result_file_saved)
            mutableState.value = mutableState.value.copy(
                awaitingSave = false,
                result = OperationResult.Success(saved, pendingSavedDetail),
            )
            pendingSavedDetail = null
        } catch (_: Exception) {
            // Un proveedor SAF puede haber creado un fichero parcial. La salida se
            // conserva solo en memoria para poder elegir otro destino sin refirmar.
            mutableState.value = mutableState.value.copy(
                result = OperationResult.Error(UiText.Resource(
                    if (signature) R.string.error_save_failed_keep_output else R.string.error_tool_save_failed,
                )),
            )
        }
    }

    fun retryPendingOutput() = launchOperation(allowAwaitingSave = true) {
        if (pendingBatch != null) {
            effectChannel.send(UiEffect.ChooseBatchFolder)
            return@launchOperation
        }
        val output = pendingOutput
            ?: throw IllegalStateException("No hay un resultado de firma pendiente.")
        effectChannel.send(UiEffect.SaveSignedDocument(output.displayName, output.mimeType))
    }

    fun discardPendingOutput() {
        if (!mutableState.value.canDiscardPendingOutput) return
        val kind = mutableState.value.pendingKind
        clearPending()
        if (kind != PendingKind.SIGNATURE) {
            mutableState.value = mutableState.value.copy(awaitingSave = false,
                result = OperationResult.Error(UiText.Resource(R.string.result_tool_discarded)))
            return
        }
        val result = try {
            core.clearSession()
            OperationResult.Error(UiText.Resource(R.string.result_save_cancelled))
        } catch (error: Exception) {
            OperationResult.Error(error.toUserText())
        }
        mutableState.value = mutableState.value.copy(
            awaitingSave = false,
            certificate = null,
            certificateExternal = false,
            result = result,
        )
    }

    private fun replacePending(output: SignedOutput, kind: PendingKind, savedDetail: UiText? = null) {
        clearPending()
        pendingOutput = output
        pendingSavedDetail = savedDetail
        mutableState.value = mutableState.value.copy(pendingKind = kind)
    }

    private fun clearPending() {
        pendingOutput?.bytes?.fill(0)
        pendingOutput = null
        pendingBatch?.forEach { it.output?.bytes?.fill(0) }
        pendingBatch = null
        pendingSavedDetail = null
    }

    fun forgetCertificate() {
        if (!mutableState.value.canForgetCertificate) return
        try {
            core.clearSession()
            mutableState.value = mutableState.value.copy(
                certificate = null,
                certificateFile = null,
                certificateExternal = false,
                result = OperationResult.Success(UiText.Resource(R.string.result_certificate_forgotten)),
            )
        } catch (error: Exception) {
            setError(error.toUserText())
        }
    }

    // --- Herramientas: huellas, protección y lote ---

    fun updateToolSettings(hashAlgorithm: String, hashFormat: String, container: String, protectForMe: Boolean) {
        if (!mutableState.value.canReplaceSelection) return
        require(hashAlgorithm in ToolsPolicy.HASH_ALGORITHMS)
        require(hashFormat in ToolsPolicy.HASH_FORMATS)
        require(container in ToolsPolicy.CONTAINERS)
        mutableState.value = mutableState.value.copy(hashAlgorithm = hashAlgorithm, hashFormat = hashFormat,
            protectionContainer = container, protectForMe = protectForMe)
    }

    fun createHash() {
        val snapshot = mutableState.value
        val document = snapshot.document ?: return setError(UiText.Resource(R.string.error_document_required))
        if (!snapshot.canHash) return
        launchOperation {
            val loaded = repository.loadDocument(document)
            try {
                val hash = core.createHash(loaded, snapshot.hashAlgorithm, snapshot.hashFormat)
                val detail = UiText.Resource(R.string.result_hash_detail, listOf(hash.algorithm, hash.hash))
                replacePending(
                    SignedOutput(hash.bytes, ToolsPolicy.hashFileName(document.displayName, hash.extension),
                        ToolsPolicy.hashMime(hash.format), hash.format, hash.algorithm),
                    PendingKind.TOOL,
                    detail,
                )
                mutableState.value = mutableState.value.copy(awaitingSave = true,
                    result = OperationResult.Success(UiText.Resource(R.string.result_hash_created), detail))
                val output = pendingOutput ?: return@launchOperation
                effectChannel.send(UiEffect.SaveSignedDocument(output.displayName, output.mimeType))
            } finally {
                loaded.bytes.fill(0)
            }
        }
    }

    fun checkHash(hashUri: Uri) {
        val snapshot = mutableState.value
        val document = snapshot.document ?: return setError(UiText.Resource(R.string.error_document_required))
        if (!snapshot.canHash) return
        launchOperation {
            val hashFile = repository.inspect(hashUri, "huella", "application/octet-stream")
            DocumentPolicy.requireAllowedSize(hashFile.sizeBytes, ToolsPolicy.MAX_HASH_FILE_BYTES, "La huella")
            val stored = repository.loadCertificate(hashFile)
            val loaded = try { repository.loadDocument(document) } catch (error: Exception) { stored.bytes.fill(0); throw error }
            try {
                if (stored.bytes.size > ToolsPolicy.MAX_HASH_FILE_BYTES) {
                    setError(UiText.Resource(R.string.error_hash_file))
                    return@launchOperation
                }
                val check = core.checkHash(loaded, stored)
                val lines = UiText.Lines(listOf(
                    UiText.Resource(R.string.hash_check_algorithm, listOf(check.algorithm)),
                    UiText.Resource(R.string.hash_expected, listOf(check.expected)),
                    UiText.Resource(R.string.hash_actual, listOf(check.actual)),
                ))
                mutableState.value = mutableState.value.copy(result = if (check.valid)
                    OperationResult.Success(UiText.Resource(R.string.result_hash_match), lines)
                else OperationResult.Error(UiText.Lines(listOf(UiText.Resource(R.string.result_hash_mismatch), lines))))
            } finally {
                loaded.bytes.fill(0)
                stored.bytes.fill(0)
            }
        }
    }

    fun addRecipient(uri: Uri) {
        if (!mutableState.value.canReplaceSelection) return reportBusyIncomingIntent()
        runInspect {
            if (mutableState.value.recipients.size >= ToolsPolicy.MAX_RECIPIENTS) {
                setError(UiText.Resource(R.string.error_recipient_too_many))
                return@runInspect
            }
            val selected = repository.inspect(uri, "destinatario.cer", "application/pkix-cert")
            DocumentPolicy.requireAllowedSize(selected.sizeBytes, ToolsPolicy.MAX_RECIPIENT_BYTES, "El certificado")
            mutableState.value = mutableState.value.copy(
                recipients = (mutableState.value.recipients + selected).distinctBy { it.uri },
                result = OperationResult.Idle,
            )
        }
    }

    fun clearRecipients() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(recipients = emptyList())
    }

    /** [secret] y [confirmation] se borran siempre, también si hay error. */
    fun protect(secret: CharArray, confirmation: CharArray, sign: Boolean) {
        val snapshot = mutableState.value
        val document = snapshot.document
        val problem: Int? = when {
            document == null -> R.string.error_document_required
            !snapshot.canProtect -> -1
            snapshot.usesTransientKey && sign -> R.string.error_protect_sign_identity
            snapshot.usesTransientKey && (!ToolsPolicy.canonicalAesKey(secret) || !secret.contentEquals(confirmation)) ->
                R.string.error_protect_key
            // Sin identidad válida para firmar (o con DNIe) se avisa antes que de
            // los destinatarios: es lo que impide la operación.
            sign && snapshot.certificate == null -> if (snapshot.externalProtectSignAvailable)
                R.string.error_protect_sign_identity_any else R.string.error_protect_sign_identity
            sign && snapshot.certificateExternal && !snapshot.externalProtectSignAvailable -> R.string.error_protect_sign_identity
            !snapshot.usesTransientKey && snapshot.recipients.isEmpty() && !snapshot.canProtectForMe ->
                R.string.error_protect_recipients_required
            else -> null
        }
        confirmation.fill('\u0000')
        if (problem != null || document == null) {
            secret.fill('\u0000')
            if (problem != null && problem != -1) setError(UiText.Resource(problem))
            return
        }
        val key = if (snapshot.usesTransientKey) secret else { secret.fill('\u0000'); null }
        launchOperation(onFinished = { key?.fill('\u0000') }) {
            val loadedRecipients = ArrayList<ByteArray>()
            val loaded = repository.loadDocument(document)
            try {
                if (!snapshot.usesTransientKey) for (recipient in snapshot.recipients) {
                    val file = repository.loadCertificate(recipient)
                    loadedRecipients += file.bytes
                    if (file.bytes.size > ToolsPolicy.MAX_RECIPIENT_BYTES) {
                        setError(UiText.Resource(R.string.error_protect_recipient))
                        return@launchOperation
                    }
                }
                val request = ProtectionRequest(
                    container = if (sign) "signedandenvelopeddata" else snapshot.protectionContainer,
                    recipients = loadedRecipients,
                    includeSessionCertificate = !snapshot.usesTransientKey && snapshot.canProtectForMe,
                    sign = sign,
                    certificateId = if (sign) snapshot.certificate?.id.orEmpty() else "",
                )
                val output = core.protect(loaded, request, key)
                val detail = UiText.Resource(R.string.result_protected_detail, listOf(output.displayName))
                replacePending(output, PendingKind.TOOL, detail)
                mutableState.value = mutableState.value.copy(awaitingSave = true,
                    result = OperationResult.Success(UiText.Resource(
                        if (sign) R.string.result_protected_signed else R.string.result_protected), detail))
                effectChannel.send(UiEffect.SaveSignedDocument(output.displayName, output.mimeType))
            } finally {
                loaded.bytes.fill(0)
                loadedRecipients.forEach { it.fill(0) }
                key?.fill('\u0000')
            }
        }
    }

    /** [secret] vacío usa el certificado PKCS#12 de la sesión; se borra siempre. */
    fun unprotect(secret: CharArray) {
        val snapshot = mutableState.value
        val document = snapshot.document
        val problem: Int? = when {
            document == null -> R.string.error_document_required
            !snapshot.canUnprotect -> -1
            secret.isNotEmpty() && !ToolsPolicy.canonicalAesKey(secret) -> R.string.error_protect_key
            secret.isEmpty() && (snapshot.certificate == null || snapshot.certificateExternal) ->
                R.string.error_unprotect_key_or_certificate
            else -> null
        }
        if (problem != null || document == null) {
            secret.fill('\u0000')
            if (problem != null && problem != -1) setError(UiText.Resource(problem))
            return
        }
        val key = secret.takeIf { it.isNotEmpty() }
        launchOperation(onFinished = { secret.fill('\u0000') }) {
            val loaded = repository.loadDocument(document)
            try {
                val output = core.unprotect(loaded, key)
                val detail = UiText.Resource(R.string.result_unprotected_detail, listOf(output.displayName))
                replacePending(output, PendingKind.TOOL, detail)
                mutableState.value = mutableState.value.copy(awaitingSave = true,
                    result = OperationResult.Success(UiText.Resource(R.string.result_unprotected), detail))
                effectChannel.send(UiEffect.SaveSignedDocument(output.displayName, output.mimeType))
            } finally {
                loaded.bytes.fill(0)
                secret.fill('\u0000')
            }
        }
    }

    fun selectBatchDocuments(uris: List<Uri>) {
        if (!mutableState.value.canReplaceSelection) return reportBusyIncomingIntent()
        if (uris.isEmpty()) return
        runInspect {
            if (uris.size > ToolsPolicy.MAX_BATCH_ITEMS) {
                setError(UiText.Resource(R.string.error_batch_too_many))
                return@runInspect
            }
            val selected = uris.distinct().map { repository.inspect(it, "documento", "application/octet-stream") }
            selected.forEach {
                DocumentPolicy.requireAllowedSize(it.sizeBytes, DocumentPolicy.MAX_DOCUMENT_BYTES, "El documento")
            }
            ToolsPolicy.batchProblem(selected.map { it.sizeBytes })?.let {
                setError(UiText.Resource(it))
                return@runInspect
            }
            mutableState.value = mutableState.value.copy(batchDocuments = selected, result = OperationResult.Idle)
        }
    }

    fun clearBatchDocuments() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(batchDocuments = emptyList())
    }

    /**
     * Firma o cofirma el lote. Con [sealPlanner], los PDF reciben el sello
     * visible calculado para cada documento. Con DNIe, [externalError] entrega
     * el primer fallo de la tarjeta para explicarlo junto al resultado, y
     * [onFinished] cierra la operación de PIN (también si no llega a empezar).
     */
    fun signBatch(
        format: String,
        sealPlanner: BatchSealPlanner? = null,
        externalError: (() -> Pair<Throwable, Int>?)? = null,
        onFinished: () -> Unit = {},
    ) {
        val snapshot = mutableState.value
        val certificate = snapshot.certificate
        val action = if (snapshot.batchCosignAvailable) snapshot.wave4.batchAction else "sign"
        val seal = sealPlanner != null && snapshot.wave4.batchSeal && snapshot.batchSealAvailable
        val problem: Int? = when {
            snapshot.batchDocuments.isEmpty() -> R.string.error_batch_required
            certificate == null -> R.string.error_certificate_required
            snapshot.certificateExternal && !snapshot.externalBatchAvailable -> R.string.batch_dnie_unavailable
            !snapshot.canSignBatch -> -1
            snapshot.batchDocuments.any { ToolsPolicy.effectiveFormat(format, it.displayName, it.mimeType) == "verifactu" } ->
                R.string.error_batch_verifactu
            snapshot.batchDocuments.any {
                val effective = ToolsPolicy.effectiveFormat(format, it.displayName, it.mimeType)
                effective !in snapshot.signingFormats || !SigningOptions.supported(effective, action, snapshot.signatureProfile)
            } -> R.string.error_signature_combination
            else -> null
        }
        if (problem != null || certificate == null) {
            onFinished()
            if (problem != null && problem != -1) setError(UiText.Resource(problem))
            return
        }
        launchOperation(onFinished = onFinished) {
            // Con formatos solo B en la selección, el lote se firma sin TSA.
            val withTimestamp = snapshot.tsaEnabled && snapshot.batchDocuments.all {
                FormatPolicy.acceptsTimestamp(ToolsPolicy.effectiveFormat(format, it.displayName, it.mimeType))
            }
            val configured = try {
                SigningOptions.create(snapshot.signatureProfile, withTimestamp, snapshot.tsaUrl)
            } catch (_: Exception) {
                setError(UiText.Resource(R.string.error_tsa_configuration))
                return@launchOperation
            }
            val loaded = ArrayList<LoadedFile>(snapshot.batchDocuments.size)
            try {
                var total = 0L
                for (document in snapshot.batchDocuments) {
                    val file = repository.loadDocument(document)
                    loaded += file
                    total += file.bytes.size
                    if (total > ToolsPolicy.MAX_BATCH_TOTAL_BYTES) {
                        setError(UiText.Resource(R.string.error_batch_too_large))
                        return@launchOperation
                    }
                }
                val results = if (action == "sign" && !seal) {
                    core.signBatch(loaded, format, certificate.id, configured)
                } else {
                    val items = ArrayList<BatchItemInput>(loaded.size)
                    for (file in loaded) {
                        val effective = ToolsPolicy.effectiveFormat(format, file.displayName, file.mimeType, file.bytes)
                        val options = if (seal && effective == "pades") {
                            sealPlanner?.options(file) ?: run {
                                setError(UiText.Resource(R.string.batch_seal_error, listOf(file.displayName)))
                                return@launchOperation
                            }
                        } else emptyMap()
                        items += BatchItemInput(file, format, action, options)
                    }
                    core.signBatchItems(items, certificate.id, configured)
                }
                clearPending()
                pendingBatch = results
                val ok = results.count { it.output != null }
                val cardProblem = externalError?.invoke()
                val lines = buildList<UiText> {
                    cardProblem?.let { (error, retries) -> add(dnieText(error, retries)) }
                    add(UiText.Plural(R.plurals.batch_ok_count, ok))
                    if (ok < results.size) add(UiText.Plural(R.plurals.batch_error_count, results.size - ok))
                    results.forEach {
                        add(UiText.Resource(if (it.output != null) R.string.batch_item_ok else R.string.batch_item_error,
                            listOf(it.sourceName)))
                    }
                }
                if (ok == 0) {
                    clearPending()
                    mutableState.value = mutableState.value.copy(result = OperationResult.Error(UiText.Lines(lines)))
                    return@launchOperation
                }
                mutableState.value = mutableState.value.copy(awaitingSave = true, pendingKind = PendingKind.BATCH,
                    result = OperationResult.Success(UiText.Resource(
                        if (action == "cosign") R.string.result_batch_cosigned else R.string.result_batch_signed),
                        UiText.Lines(lines)))
                effectChannel.send(UiEffect.ChooseBatchFolder)
            } finally {
                loaded.forEach { it.bytes.fill(0) }
            }
        }
    }

    fun updateBatchOptions(action: String, seal: Boolean) {
        if (!mutableState.value.canReplaceSelection) return
        require(action in listOf("sign", "cosign"))
        val current = mutableState.value
        mutableState.value = current.copy(wave4 = current.wave4.copy(batchAction = action, batchSeal = seal))
    }

    fun saveBatchOutputs(folder: Uri) = launchOperation(allowAwaitingSave = true) {
        val batch = pendingBatch ?: throw IllegalStateException("No hay un lote pendiente.")
        val lines = ArrayList<UiText>()
        var saved = 0
        var failed = 0
        for (item in batch) {
            val output = item.output
            if (output == null) {
                lines += UiText.Resource(R.string.batch_item_error, listOf(item.sourceName))
                continue
            }
            if (output.bytes.isEmpty()) {
                saved++
                lines += UiText.Resource(R.string.batch_item_saved, listOf(output.displayName))
                continue
            }
            try {
                repository.writeToTree(folder, output.displayName, output.mimeType, output.bytes)
                output.bytes.fill(0)
                saved++
                lines += UiText.Resource(R.string.batch_item_saved, listOf(output.displayName))
            } catch (_: Exception) {
                failed++
                lines += UiText.Resource(R.string.batch_item_not_saved, listOf(output.displayName))
            }
        }
        if (failed == 0) {
            clearPending()
            mutableState.value = mutableState.value.copy(awaitingSave = false, result = OperationResult.Success(
                UiText.Plural(R.plurals.batch_saved_count, saved), UiText.Lines(lines)))
        } else {
            // Las firmas no guardadas siguen en memoria para elegir otra carpeta.
            mutableState.value = mutableState.value.copy(result = OperationResult.Error(UiText.Lines(
                listOf(UiText.Resource(R.string.error_tool_save_failed)) + lines)))
        }
    }

    // --- Veri*Factu, ENI y leyenda CSV ---

    suspend fun csvLegend(code: String, url: String, text: String): CsvLegend =
        withContext(ioDispatcher) { core.csvLegend(code, url, text) }

    fun eniCatalogs(): EniCatalogs = try { core.eniCatalogs() } catch (_: Exception) { SignatureFormats.DEFAULT_ENI_CATALOGS }

    fun selectVeriFactuRecords(uris: List<Uri>) {
        if (!mutableState.value.canReplaceSelection) return reportBusyIncomingIntent()
        if (uris.isEmpty()) return
        runInspect {
            if (uris.size > ToolsPolicy.MAX_VERIFACTU_FILES) {
                setError(UiText.Resource(R.string.error_verifactu_too_many))
                return@runInspect
            }
            val selected = uris.distinct().map { repository.inspect(it, "registro.xml", "application/xml") }
            ToolsPolicy.veriFactuProblem(selected.map { it.sizeBytes })?.let {
                setError(UiText.Resource(it))
                return@runInspect
            }
            mutableState.value = mutableState.value.copy(verifactuRecords = selected, result = OperationResult.Idle)
        }
    }

    fun clearVeriFactuRecords() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(verifactuRecords = emptyList())
    }

    /** Comprueba los registros sin consultar a la AEAT y muestra el informe por registro. */
    fun checkVeriFactu() {
        val snapshot = mutableState.value
        if (!snapshot.canCheckVeriFactu) return
        launchOperation {
            val loaded = ArrayList<LoadedFile>(snapshot.verifactuRecords.size)
            try {
                var total = 0L
                for (record in snapshot.verifactuRecords) {
                    val file = repository.loadBounded(record, ToolsPolicy.MAX_VERIFACTU_FILE_BYTES)
                    loaded += file
                    total += file.bytes.size
                    if (total > ToolsPolicy.MAX_VERIFACTU_TOTAL_BYTES) {
                        setError(UiText.Resource(R.string.error_verifactu_too_many))
                        return@launchOperation
                    }
                }
                val report = core.validateVeriFactu(loaded)
                val lines = VeriFactuText.lines(report)
                mutableState.value = mutableState.value.copy(result = if (report.valid)
                    OperationResult.Success(UiText.Resource(R.string.verifactu_result_valid), lines)
                else OperationResult.Error(UiText.Lines(listOf(UiText.Resource(R.string.verifactu_result_invalid), lines))))
            } finally {
                loaded.forEach { it.bytes.fill(0) }
            }
        }
    }

    fun updateEniCaptureDate(utcMidnight: Long?) {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(eniCaptureDate = utcMidnight)
    }

    /**
     * Crea el documento ENI con la firma elegida como documento y, si la firma
     * es separada, con el documento original. El resultado se guarda por SAF.
     */
    fun createEni(request: EniRequest) {
        val snapshot = mutableState.value
        val document = snapshot.document ?: return setError(UiText.Resource(R.string.error_document_required))
        if (!snapshot.canCreateEni) return
        if (!EniForm.organsValid(request.organs)) return setError(UiText.Engine("eni.validacion.dir3"))
        launchOperation {
            val signature = repository.loadBounded(document, DocumentPolicy.MAX_SIGNED_OUTPUT_BYTES)
            var original: LoadedFile? = null
            try {
                original = snapshot.originalDocument?.let(repository::loadDocument)
                val created = core.createEniDocument(signature, original, request)
                val name = EniForm.outputName(document.displayName)
                val detail = UiText.Resource(R.string.eni_created_detail, listOf(created.signatureType, name))
                replacePending(SignedOutput(created.bytes, name, "application/xml", "ENI", created.signatureType),
                    PendingKind.TOOL, detail)
                mutableState.value = mutableState.value.copy(awaitingSave = true,
                    result = OperationResult.Success(UiText.Resource(R.string.eni_created), detail))
                effectChannel.send(UiEffect.SaveSignedDocument(name, "application/xml"))
            } finally {
                signature.bytes.fill(0)
                original?.bytes?.fill(0)
            }
        }
    }

    /** Revisa la estructura de un ENI elegido por SAF; no verifica sus firmas. */
    fun validateEni(uri: Uri) {
        if (!mutableState.value.canValidateEni) return
        launchOperation {
            val selected = repository.inspect(uri, "documento-eni.xml", "application/xml")
            DocumentPolicy.requireAllowedSize(selected.sizeBytes, DocumentPolicy.MAX_DOCUMENT_BYTES, "El documento")
            val loaded = repository.loadDocument(selected)
            try {
                val validation = core.validateEni(loaded)
                val issues = validation.issues.map { UiText.Resource(R.string.issue_line, listOf(it.field, UiText.Engine(it.key))) }
                mutableState.value = mutableState.value.copy(result = if (validation.valid)
                    OperationResult.Success(UiText.Engine("eni.validacion.valid"),
                        UiText.Resource(R.string.eni_validate_scope))
                else OperationResult.Error(UiText.Lines(
                    listOf(UiText.Resource(R.string.eni_invalid, listOf(selected.displayName))) + issues +
                        UiText.Resource(R.string.eni_validate_scope))))
            } finally {
                loaded.bytes.fill(0)
            }
        }
    }

    // --- Expediente ENI (cuarta oleada) ---

    /** Lista los documentos XML de la carpeta elegida; el permiso no se persiste. */
    fun selectEniFileFolder(folder: Uri) {
        if (!mutableState.value.canReplaceSelection) return reportBusyIncomingIntent()
        launchOperation {
            val entries = repository.listTree(folder, ExpedientePolicy.MAX_FOLDER_ENTRIES)
            val (documents, skipped) = ExpedientePolicy.partition(entries)
            ExpedientePolicy.selectionProblem(documents.map { it.sizeBytes })?.let {
                setError(UiText.Resource(it))
                return@launchOperation
            }
            val current = mutableState.value
            mutableState.value = current.copy(result = OperationResult.Idle,
                wave4 = current.wave4.copy(eniFileDocuments = documents, eniFileSkipped = skipped))
        }
    }

    fun clearEniFileDocuments() {
        if (!mutableState.value.canReplaceSelection) return
        val current = mutableState.value
        mutableState.value = current.copy(wave4 = current.wave4.copy(eniFileDocuments = emptyList(), eniFileSkipped = 0))
    }

    fun updateEniFileOpeningDate(utcMidnight: Long?) {
        if (!mutableState.value.canReplaceSelection) return
        val current = mutableState.value
        mutableState.value = current.copy(wave4 = current.wave4.copy(eniFileOpeningDate = utcMidnight))
    }

    /**
     * Crea el expediente con los documentos de la carpeta y firma su índice con
     * el certificado de la sesión. Con DNIe, [onFinished] borra el PIN.
     */
    fun createEniFile(request: EniFileRequest, onFinished: () -> Unit = {}) {
        val snapshot = mutableState.value
        val certificate = snapshot.certificate
        val problem: UiText? = when {
            !snapshot.eniFileAvailable -> UiText.Resource(R.string.error_core_unavailable)
            certificate == null -> UiText.Resource(R.string.error_certificate_required)
            snapshot.wave4.eniFileDocuments.isEmpty() -> UiText.Resource(R.string.expediente_error_no_xml)
            !EniForm.organsValid(request.organs) -> UiText.Engine("eni.validacion.dir3")
            !ExpedientePolicy.classificationValid(request.classification) -> UiText.Engine("eni.validacion.classification")
            request.state !in ExpedientePolicy.STATES -> UiText.Engine("eni.validacion.value")
            !ExpedientePolicy.identifierValid(request.identifier) -> UiText.Engine("eni.validacion.identifier")
            !ExpedientePolicy.interestedValid(request.interested) -> UiText.Engine("eni.validacion.text")
            !snapshot.canCreateEniFile -> UiText.Resource(R.string.error_operation_in_progress)
            else -> null
        }
        if (problem != null || certificate == null) {
            onFinished()
            problem?.let(::setError)
            return
        }
        launchOperation(onFinished = onFinished) {
            val loaded = ArrayList<LoadedFile>(snapshot.wave4.eniFileDocuments.size)
            try {
                var total = 0L
                for (document in snapshot.wave4.eniFileDocuments) {
                    val file = repository.loadBounded(document, DocumentPolicy.MAX_DOCUMENT_BYTES)
                    loaded += file
                    total += file.bytes.size
                    if (total > ExpedientePolicy.MAX_TOTAL_BYTES) {
                        setError(UiText.Resource(R.string.expediente_error_too_large))
                        return@launchOperation
                    }
                }
                val created = core.createEniFile(loaded, certificate.id, request)
                val bytes = created.bytes
                if (bytes == null) {
                    mutableState.value = mutableState.value.copy(result = OperationResult.Error(UiText.Lines(
                        listOf(UiText.Resource(R.string.expediente_invalid_documents)) +
                            created.issues.map { UiText.Resource(R.string.issue_line, listOf(it.field, UiText.Engine(it.key))) })))
                    return@launchOperation
                }
                val name = ExpedientePolicy.outputName(request.identifier)
                val detail = UiText.Lines(listOf(
                    UiText.Plural(R.plurals.expediente_documents_count, created.documents),
                    UiText.Resource(R.string.expediente_output_name, listOf(name)),
                ))
                replacePending(SignedOutput(bytes, name, "application/xml", "ENI", "XAdES"), PendingKind.TOOL, detail)
                mutableState.value = mutableState.value.copy(awaitingSave = true,
                    result = OperationResult.Success(UiText.Resource(R.string.expediente_created), detail))
                effectChannel.send(UiEffect.SaveSignedDocument(name, "application/xml"))
            } finally {
                loaded.forEach { it.bytes.fill(0) }
            }
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
        clearPending()
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
