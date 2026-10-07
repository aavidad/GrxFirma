// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreBridge
import io.github.aavidad.grxfirma.android.core.ExternalIdentityBridge
import io.github.aavidad.grxfirma.android.nfc.DnieError
import io.github.aavidad.grxfirma.android.nfc.DnieErrors
import io.github.aavidad.grxfirma.android.nfc.DnieNfcSession
import io.github.aavidad.grxfirma.android.files.DocumentRepository
import io.github.aavidad.grxfirma.android.files.DocumentPolicy
import io.github.aavidad.grxfirma.android.model.SignedOutput
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SignatureInspection
import io.github.aavidad.grxfirma.android.model.BatchItemResult
import io.github.aavidad.grxfirma.android.model.ProtectionRequest
import io.github.aavidad.grxfirma.android.model.CsvLegend
import io.github.aavidad.grxfirma.android.model.EniCatalogs
import io.github.aavidad.grxfirma.android.model.EniRequest
import io.github.aavidad.grxfirma.android.model.EniFileRequest
import io.github.aavidad.grxfirma.android.model.BatchItemInput
import io.github.aavidad.grxfirma.android.core.SignatureFormats
import io.github.aavidad.grxfirma.android.core.AppLinks
import io.github.aavidad.grxfirma.android.core.PlatformServices
import io.github.aavidad.grxfirma.android.model.CertificateDetail
import io.github.aavidad.grxfirma.android.qr.QrImagePreparer
import io.github.aavidad.grxfirma.android.settings.AppSettings
import io.github.aavidad.grxfirma.android.settings.AppSettingsStore
import io.github.aavidad.grxfirma.android.settings.InMemorySettingsStore
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
    private val settingsStore: AppSettingsStore = InMemorySettingsStore(),
) : ViewModel() {
    private val initialSettings = settingsStore.load()
    private val mutableState = MutableStateFlow(
        MainUiState(
            backend = core.readiness,
            toolsAvailable = core.readiness.available && core.toolsAvailable,
            signingFormats = core.signingFormats,
            documentServices = if (core.readiness.available) core.documentServices else emptySet(),
            platformServices = if (core.readiness.available) core.platformServices else emptySet(),
            settings = initialSettings,
            signatureProfile = initialSettings.defaultProfile,
            tsaEnabled = initialSettings.tsaEnabled,
            tsaUrl = initialSettings.tsaUrl,
            capabilities = if (core.readiness.available) core.capabilities else emptySet(),
            maxIdentities = if (core.readiness.available) core.maxIdentities.coerceAtLeast(1) else 1,
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
                DocumentPolicy.MAX_DOCUMENT_BYTES)
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
                DocumentPolicy.MAX_DOCUMENT_BYTES)
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
                DocumentPolicy.MAX_CERTIFICATE_BYTES)
            mutableState.value = mutableState.value.clearedResult().copy(certificateFile = selected, certificatePasswordError = null)
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
        return withContext(ioDispatcher) { core.sealPreview(certificate.id, mutableState.value.settings.withSealLanguage(options)) }
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
            mutableState.value = mutableState.value.copy(
                certificatePasswordError = UiText.Resource(R.string.error_password_required))
            return
        }
        mutableState.value = mutableState.value.copy(certificatePasswordError = null)
        launchOperation(onFinished = { password.fill('\u0000') }) {
            try {
                val loaded = repository.loadCertificate(selected)
                try {
                    val certificate = try {
                        core.importCertificate(loaded.bytes, password)
                    } catch (error: Exception) {
                        val text = error.toUserText()
                        // Contraseña mala: el aviso va en su campo, no en la tarjeta de resultado.
                        if (text == UiText.Resource(R.string.error_pkcs12_password_or_legacy)) {
                            mutableState.value = mutableState.value.copy(certificatePasswordError = text)
                            return@launchOperation
                        }
                        throw error
                    }
                    val details = loadCertificateDetails()
                    mutableState.value = mutableState.value.withIdentity(SessionIdentity(certificate, external = false)).copy(
                        certificateDetails = details.first,
                        expiringSoonDays = details.second,
                        certificateFile = null,
                        result = OperationResult.Success(
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
                val details = loadCertificateDetails()
                withContext(Dispatchers.Main) {
                    if (identityEpoch.get() != epoch) {
                        // La lectura terminó cuando ya no hacía falta: se cierra solo ese DNIe.
                        if (mutableState.value.canKeepSeveralIdentities) {
                            try { core.removeIdentity(certificate.id) } catch (_: Exception) { closeAllIdentities() }
                        } else core.clearSession()
                        session.close()
                    } else {
                        mutableState.value = mutableState.value.withIdentity(SessionIdentity(certificate, external = true)).copy(
                            certificateDetails = details.first,
                            expiringSoonDays = details.second,
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

    /**
     * La lectura del DNIe ha terminado (salir de la app, tarjeta retirada).
     * Con varias identidades se cierra solo el DNIe y siguen abiertos los
     * PKCS#12; con un núcleo de una sola identidad se cierra la sesión.
     */
    fun clearDnieIdentity() {
        identityEpoch.incrementAndGet()
        val current = mutableState.value
        val dnie = current.identities.firstOrNull { it.external }
        val keepOthers = current.canKeepSeveralIdentities && dnie != null && current.identities.any { !it.external }
        val closedOnlyDnie = keepOthers && try {
            core.removeIdentity(dnie!!.id)
            true
        } catch (_: Exception) { false }
        if (closedOnlyDnie) {
            val remaining = SessionIdentities.remove(current.identities, dnie!!.id)
            val selected = SessionIdentities.selectionAfter(remaining, current.certificate?.id)
            mutableState.value = current.copy(
                identities = remaining,
                certificate = selected?.certificate,
                certificateExternal = selected?.external == true,
                certificateDetails = loadCertificateDetails().first,
            )
        } else {
            closeAllIdentities()
        }
        val after = mutableState.value
        mutableState.value = after.copy(
            result = if (current.awaitingSave) current.result else
                OperationResult.Error(UiText.Resource(R.string.dnie_session_ended)),
        )
    }

    /** Elige con qué certificado abierto se firma. */
    fun selectIdentity(certificateId: String) {
        val snapshot = mutableState.value
        if (!snapshot.canChangeIdentity) return
        val identity = snapshot.identities.firstOrNull { it.id == certificateId } ?: return
        if (identity.id == snapshot.certificate?.id) return
        mutableState.value = snapshot.copy(certificate = identity.certificate, certificateExternal = identity.external)
    }

    /** Cierra uno de los certificados abiertos; los demás siguen disponibles. */
    fun closeIdentity(certificateId: String) {
        val snapshot = mutableState.value
        if (!snapshot.canChangeIdentity) return
        val identity = snapshot.identities.firstOrNull { it.id == certificateId } ?: return
        if (!snapshot.canKeepSeveralIdentities || snapshot.identities.size == 1) {
            forgetCertificate()
            return
        }
        if (identity.external) identityEpoch.incrementAndGet()
        try {
            core.removeIdentity(certificateId)
            val remaining = SessionIdentities.remove(snapshot.identities, certificateId)
            val selected = SessionIdentities.selectionAfter(remaining, snapshot.certificate?.id)
            mutableState.value = mutableState.value.copy(
                identities = remaining,
                certificate = selected?.certificate,
                certificateExternal = selected?.external == true,
                certificateDetails = loadCertificateDetails().first,
            ).clearedResult(OperationResult.Success(
                UiText.Resource(R.string.result_identity_closed, listOf(identity.certificate.subject)),
            ))
        } catch (error: Exception) {
            setError(error.toUserText())
        }
    }

    private fun MainUiState.withIdentity(identity: SessionIdentity): MainUiState = copy(
        identities = SessionIdentities.add(identities, identity, maxIdentities),
        certificate = identity.certificate,
        certificateExternal = identity.external,
    )

    /** Cierra todos los certificados en el núcleo y en la pantalla. */
    private fun closeAllIdentities() {
        try { core.clearSession() } catch (_: Exception) { }
        mutableState.value = mutableState.value.copy(
            identities = emptyList(),
            certificate = null,
            certificateDetails = emptyList(),
            certificateExternal = false,
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
                val output = core.sign(loaded, effectiveFormat, certificate.id,
                    snapshot.settings.withSealLanguage(options) + configured, snapshot.signatureAction)
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
                    verificationOrigin = VerificationOrigin.SIGN,
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
            var original: io.github.aavidad.grxfirma.android.model.LoadedFile? = null
            try {
                original = snapshot.originalDocument?.let(repository::loadDocument)
                val verification = core.verify(loaded, original)
                mutableState.value = mutableState.value.copy(
                    verification = verification,
                    verificationOrigin = VerificationOrigin.VERIFY,
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
        val previous = mutableState.value
        // La fecha y hora certificadas y el perfil van juntos: T, LT y LTA la
        // necesitan y B no la lleva. Se ajusta lo que la persona no ha tocado.
        var chosenProfile = profile
        var withTimestamp = tsaEnabled
        if (tsaEnabled != previous.tsaEnabled) {
            chosenProfile = if (!tsaEnabled) "baseline" else if (profile == "baseline") "t" else profile
        } else if (profile != previous.signatureProfile) {
            withTimestamp = profile != "baseline"
        }
        // Al activarla con el campo vacío se propone el servicio de Preferencias.
        val url = if (withTimestamp && tsaUrl.isBlank()) previous.settings.tsaUrl else tsaUrl
        mutableState.value = previous.copy(signatureAction = action, signatureProfile = chosenProfile,
            tsaEnabled = withTimestamp, tsaUrl = url.take(2048))
    }

    fun acceptCoSignSuggestion() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(signatureAction = "cosign", coSignSuggested = false)
    }

    private fun inspectExistingSignature(document: io.github.aavidad.grxfirma.android.model.SelectedFile) {
        if (!core.readiness.available) return
        launchOperation(clearsResult = false) {
            val loaded = repository.loadDocument(document)
            try {
                val inspection = try { core.inspectSignature(loaded) } catch (_: Exception) { SignatureInspection(false) }
                mutableState.value = mutableState.value.copy(coSignSuggested = inspection.hasSignature,
                    detectedSignatureFormat = inspection.format)
            } finally { loaded.bytes.fill(0) }
        }
    }

    /** [printable] guarda el informe HTML de escritorio en lugar del JSON técnico. */
    fun exportVerificationReport(printable: Boolean = false) {
        if (!mutableState.value.canExportReport) return
        if (printable && mutableState.value.verification?.reportHtml.isNullOrEmpty()) return
        mutableState.value = mutableState.value.copy(awaitingReportSave = true,
            reportKind = if (printable) ReportKind.VERIFICATION_HTML else ReportKind.VERIFICATION)
        viewModelScope.launch {
            effectChannel.send(if (printable) UiEffect.SaveVerificationHtml else UiEffect.SaveVerificationReport)
        }
    }

    fun cancelReportExport() {
        if (!mutableState.value.awaitingReportSave) return
        mutableState.value = mutableState.value.copy(awaitingReportSave = false,
            result = OperationResult.Error(UiText.Resource(R.string.error_report_export)))
    }

    fun saveVerificationReport(uri: Uri) {
        val kind = mutableState.value.reportKind
        if (!mutableState.value.awaitingReportSave || (kind != ReportKind.VERIFICATION && kind != ReportKind.VERIFICATION_HTML)) return
        val verification = mutableState.value.verification ?: return cancelReportExport()
        val report = if (kind == ReportKind.VERIFICATION_HTML) verification.reportHtml else verification.reportJson
        if (report.isEmpty()) return cancelReportExport()
        mutableState.value = mutableState.value.copy(awaitingReportSave = false)
        launchOperation(clearsResult = false) {
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
            // Si ya existía un fichero con ese nombre, Android guarda otro («… (1).pdf»):
            // se muestra el nombre real, no el propuesto.
            val savedName = repository.savedDisplayName(uri)
            val detail = if (savedName == null) pendingSavedDetail
                else pendingSavedDetail?.replacingArgument(output.displayName, savedName)
            mutableState.value = mutableState.value.copy(
                awaitingSave = false,
                result = OperationResult.Success(saved, detail),
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
                result = OperationResult.Notice(UiText.Resource(R.string.result_tool_discarded)))
            return
        }
        val result = try {
            core.clearSession()
            OperationResult.Notice(UiText.Resource(R.string.result_save_cancelled))
        } catch (error: Exception) {
            OperationResult.Error(error.toUserText())
        }
        mutableState.value = mutableState.value.copy(
            awaitingSave = false,
            identities = emptyList(),
            certificate = null,
            certificateDetails = emptyList(),
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

    /**
     * Cierra todos los certificados abiertos si la app ha pasado en segundo
     * plano el tiempo elegido (reloj monótono `SystemClock.elapsedRealtime`).
     * El DNIe ya se cierra al salir de la pantalla. Si el plazo ha vencido pero
     * hay una operación en curso o un resultado sin guardar, devuelve POSTPONED
     * para que se vuelva a comprobar más tarde: nunca se abandona la comprobación.
     */
    fun closeCertificateAfterBackground(elapsedMillis: Long): AutoClose {
        val snapshot = mutableState.value
        val minutes = snapshot.settings.sessionTimeoutMinutes
        if ((snapshot.certificate == null && snapshot.identities.isEmpty()) || minutes <= 0) return AutoClose.NOT_APPLICABLE
        if (elapsedMillis < minutes * 60_000L) return AutoClose.NOT_DUE
        if (snapshot.busy || snapshot.awaitingSave || snapshot.awaitingReportSave) return AutoClose.POSTPONED
        identityEpoch.incrementAndGet()
        closeAllIdentities()
        mutableState.value = mutableState.value.copy(certificateFile = null)
            .clearedResult(OperationResult.Notice(UiText.Resource(R.string.certificate_closed_timeout)))
        return AutoClose.CLOSED
    }

    /** Cierra todos los certificados abiertos (con uno solo, ese). */
    fun forgetCertificate() {
        if (!mutableState.value.canForgetCertificate) return
        val several = mutableState.value.identities.size > 1
        identityEpoch.incrementAndGet()
        try {
            core.clearSession()
            mutableState.value = mutableState.value.copy(
                identities = emptyList(),
                certificate = null,
                certificateDetails = emptyList(),
                certificateFile = null,
                certificateExternal = false,
            ).clearedResult(OperationResult.Success(UiText.Resource(
                if (several) R.string.result_certificates_forgotten else R.string.result_certificate_forgotten)))
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
                val detail = UiText.Resource(R.string.result_hash_detail,
                    listOf(hash.algorithm, ToolsPolicy.displayHash(hash.format, hash.hash)))
                replacePending(
                    SignedOutput(hash.bytes, ToolsPolicy.hashFileName(document.displayName, hash.extension),
                        ToolsPolicy.HASH_MIME, hash.format, hash.algorithm),
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
            DocumentPolicy.requireAllowedSize(hashFile.sizeBytes, ToolsPolicy.MAX_HASH_FILE_BYTES)
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
            DocumentPolicy.requireAllowedSize(selected.sizeBytes, ToolsPolicy.MAX_RECIPIENT_BYTES)
            mutableState.value = mutableState.value.clearedResult().copy(
                recipients = (mutableState.value.recipients + selected).distinctBy { it.uri },
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
            snapshot.usesTransientKey && ToolsPolicy.transientKeyRejected(secret, confirmation) ->
                R.string.error_protect_key
            // Sin identidad válida para firmar (o con DNIe) se avisa antes que de
            // los destinatarios: es lo que impide la operación.
            sign && snapshot.certificate == null -> if (snapshot.externalProtectSignAvailable)
                R.string.error_protect_sign_identity_any else R.string.error_protect_sign_identity
            sign && snapshot.certificateExternal && !snapshot.externalProtectSignAvailable -> R.string.error_protect_sign_identity
            !snapshot.usesTransientKey && snapshot.recipients.isEmpty() && !snapshot.canProtectForMe ->
                R.string.error_protect_recipients_required
            !snapshot.usesTransientKey && snapshot.ownCertificateCannotEncrypt -> R.string.error_protect_own_certificate
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
                    // El núcleo usa este certificado para firmar y para «cifrar también para mí».
                    certificateId = if (sign || (!snapshot.usesTransientKey && snapshot.canProtectForMe))
                        snapshot.certificate?.id.orEmpty() else "",
                )
                val output = try {
                    core.protect(loaded, request, key)
                } catch (error: io.github.aavidad.grxfirma.android.core.CoreContractException) {
                    // Sin destinatarios, el único certificado que puede fallar es el propio.
                    if (error.toUserText() == UiText.Resource(R.string.error_protect_recipient) &&
                        request.includeSessionCertificate && loadedRecipients.isEmpty()) {
                        setError(UiText.Resource(R.string.error_protect_own_certificate))
                        return@launchOperation
                    }
                    throw error
                }
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
            !snapshot.unprotectSupported -> R.string.error_unprotect_not_protected
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
                DocumentPolicy.requireAllowedSize(it.sizeBytes, DocumentPolicy.MAX_DOCUMENT_BYTES)
            }
            ToolsPolicy.batchProblem(selected.map { it.sizeBytes })?.let {
                setError(UiText.Resource(it))
                return@runInspect
            }
            mutableState.value = mutableState.value.clearedResult().copy(batchDocuments = selected)
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
                            sealPlanner?.options(file)?.let(snapshot.settings::withSealLanguage) ?: run {
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
                batchSavedText(saved, repository.treeDisplayName(folder)), UiText.Lines(lines)))
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
            mutableState.value = mutableState.value.clearedResult().copy(verifactuRecords = selected, veriFactuReport = null)
        }
    }

    fun clearVeriFactuRecords() {
        if (!mutableState.value.canReplaceSelection) return
        mutableState.value = mutableState.value.copy(verifactuRecords = emptyList(), veriFactuReport = null)
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
                mutableState.value = mutableState.value.copy(veriFactuReport = report, result = if (report.valid)
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
                val detail = UiText.Resource(R.string.eni_created_detail, listOf(name))
                // Tras guardarlo ya no se pide elegir destino.
                replacePending(SignedOutput(created.bytes, name, "application/xml", "ENI", created.signatureType),
                    PendingKind.TOOL, UiText.Resource(R.string.eni_saved_detail, listOf(name)))
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
            DocumentPolicy.requireAllowedSize(selected.sizeBytes, DocumentPolicy.MAX_DOCUMENT_BYTES)
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

    // --- Tercera oleada: certificado, preferencias, diagnóstico, QR y versiones ---

    /** Detalle de los certificados de la sesión; vacío si el AAR no lo ofrece. */
    private fun loadCertificateDetails(): Pair<List<CertificateDetail>, Int> {
        if (PlatformServices.CERTIFICATE_DETAILS !in mutableState.value.platformServices) return emptyList<CertificateDetail>() to 30
        return try {
            core.certificateDetails().let { it.certificates to it.expiringSoonDays }
        } catch (_: Exception) {
            emptyList<CertificateDetail>() to 30
        }
    }

    fun updateCertificateFilter(query: String, kind: String) {
        require(kind.isEmpty() || kind in CertificateFilter.KINDS)
        mutableState.value = mutableState.value.copy(certificateFilter = query.take(128), certificateKindFilter = kind)
    }

    /** Consulta OCSP/CRL del certificado de la sesión, solo al pulsar el botón. */
    fun checkCertificateOnline() {
        val snapshot = mutableState.value
        val certificate = snapshot.certificate ?: return setError(UiText.Resource(R.string.error_certificate_required))
        if (!snapshot.canCheckCertificateOnline) return
        launchOperation {
            val check = core.checkCertificateRevocation(certificate.id)
            val lines = RevocationText.lines(check)
            mutableState.value = mutableState.value.copy(result = if (check.status == "valid")
                OperationResult.Success(UiText.Resource(R.string.revocation_title), lines)
            else OperationResult.Error(lines))
        }
    }

    /**
     * Guarda las preferencias y aplica las de firma a la sesión actual. Un
     * perfil con sello de tiempo exige una TSA válida.
     */
    fun savePreferences(settings: AppSettings): Boolean {
        if (!mutableState.value.canReplaceSelection) return false
        val clean = settings.sanitized()
        val valid = try {
            SigningOptions.create(clean.defaultProfile, clean.tsaEnabled, clean.tsaUrl)
            true
        } catch (_: Exception) { false }
        if (!valid) {
            setError(UiText.Resource(R.string.error_tsa_configuration))
            return false
        }
        settingsStore.save(clean)
        mutableState.value = mutableState.value.copy(settings = clean, signatureProfile = clean.defaultProfile,
            tsaEnabled = clean.tsaEnabled, tsaUrl = clean.tsaUrl,
        ).clearedResult(OperationResult.Success(UiText.Resource(R.string.preferences_saved)))
        return true
    }

    /** Vuelve a los valores de fábrica; el sello lo limpia la actividad. */
    fun restoreDefaultPreferences() {
        if (!mutableState.value.canReplaceSelection) return
        settingsStore.reset()
        val defaults = settingsStore.load()
        mutableState.value = mutableState.value.copy(settings = defaults, signatureProfile = defaults.defaultProfile,
            tsaEnabled = defaults.tsaEnabled, tsaUrl = defaults.tsaUrl,
        ).clearedResult(OperationResult.Success(UiText.Resource(R.string.preferences_restored)))
    }

    private var region: Pair<String, String>? = null

    /**
     * Pasa al núcleo el idioma de la app y la zona del móvil para el sello y el
     * informe. Solo si cambian; con una zona que el núcleo no reconoce se
     * conserva al menos el idioma (la hora sale entonces en UTC).
     */
    fun updateRegion(language: String, timeZone: String) {
        val wanted = language to timeZone
        if (wanted == region) return
        region = wanted
        try {
            core.setRegion(language, timeZone)
        } catch (_: Exception) {
            try { core.setRegion(language, "") } catch (_: Exception) { }
        }
    }

    fun loadDiagnostics() {
        if (!mutableState.value.diagnosticsAvailable) return
        val diagnostics = try { core.diagnostics() } catch (_: Exception) { null }
        mutableState.value = mutableState.value.copy(diagnostics = diagnostics)
    }

    /** Pide un sello de tiempo de prueba a la TSA configurada en la sesión. */
    fun probeTsa() {
        val snapshot = mutableState.value
        if (!snapshot.canProbeTsa) return
        mutableState.value = snapshot.copy(tsaProbe = null)
        launchOperation(clearsResult = false) {
            val probe = core.probeTimestampAuthority(snapshot.tsaUrl)
            mutableState.value = mutableState.value.copy(tsaProbe = probe)
        }
    }

    fun readVeriFactuQr(url: String) {
        if (!mutableState.value.canReadQr) return
        val trimmed = url.trim()
        mutableState.value = mutableState.value.copy(veriFactuQr = null, aeatResponse = "", qrError = null)
        if (trimmed.isEmpty()) {
            mutableState.value = mutableState.value.copy(qrError = UiText.Engine("verifactu.qr_url"))
            return
        }
        launchOperation {
            try {
                val qr = core.readVeriFactuQr(trimmed)
                mutableState.value = mutableState.value.copy(veriFactuQr = qr)
            } catch (error: Exception) {
                mutableState.value = mutableState.value.copy(qrError = error.toUserText())
            }
        }
    }

    /**
     * Lee el QR tributario de una imagen: una foto de la cámara o una imagen
     * elegida con SAF. [prepare] la reduce o convierte si hace falta; los bytes
     * se borran siempre y [onFinished] borra la foto temporal.
     */
    fun readVeriFactuQrImage(
        uri: Uri,
        prepare: (ByteArray) -> ByteArray = { it },
        onFinished: () -> Unit = {},
    ) {
        if (!mutableState.value.canReadQrImage) {
            onFinished()
            return
        }
        mutableState.value = mutableState.value.copy(veriFactuQr = null, aeatResponse = "", qrError = null)
        launchOperation(onFinished = onFinished) {
            var source: ByteArray? = null
            var prepared: ByteArray? = null
            try {
                val selected = repository.inspect(uri, "qr.jpg", "image/jpeg")
                DocumentPolicy.requireAllowedSize(selected.sizeBytes, QrImagePreparer.MAX_SOURCE_BYTES)
                source = repository.loadBounded(selected, QrImagePreparer.MAX_SOURCE_BYTES).bytes
                prepared = prepare(source)
                val qr = core.readVeriFactuQrImage(prepared)
                mutableState.value = mutableState.value.copy(veriFactuQr = qr)
            } catch (error: Exception) {
                mutableState.value = mutableState.value.copy(qrError = error.toUserText())
            } finally {
                source?.fill(0)
                prepared?.fill(0)
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
            mutableState.value = current.clearedResult().copy(
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
                replacePending(SignedOutput(bytes, name, "application/xml", "ENI", "XAdES"), PendingKind.TOOL,
                    UiText.Lines(listOf(UiText.Plural(R.plurals.expediente_documents_count, created.documents),
                        UiText.Resource(R.string.expediente_saved_detail, listOf(name)))))
                mutableState.value = mutableState.value.copy(awaitingSave = true,
                    result = OperationResult.Success(UiText.Resource(R.string.expediente_created), detail))
                effectChannel.send(UiEffect.SaveSignedDocument(name, "application/xml"))
            } finally {
                loaded.forEach { it.bytes.fill(0) }
            }
        }
    }

    /** Envía a la AEAT los cuatro datos del QR ya leído; nunca otra URL. */
    fun queryAeat() {
        val snapshot = mutableState.value
        val qr = snapshot.veriFactuQr ?: return
        if (!snapshot.canQueryAeat) return
        mutableState.value = snapshot.copy(aeatResponse = "", qrError = null)
        launchOperation {
            try {
                val response = core.queryVeriFactuQr(qr.url)
                mutableState.value = mutableState.value.copy(aeatResponse = response)
            } catch (error: Exception) {
                mutableState.value = mutableState.value.copy(qrError = error.toUserText())
            }
        }
    }

    fun clearVeriFactuQr() {
        mutableState.value = mutableState.value.copy(veriFactuQr = null, aeatResponse = "", qrError = null)
    }

    fun exportVeriFactuReport() {
        if (!mutableState.value.canExportVeriFactu) return
        mutableState.value = mutableState.value.copy(awaitingReportSave = true, reportKind = ReportKind.VERIFACTU)
        viewModelScope.launch { effectChannel.send(UiEffect.SaveVeriFactuReport) }
    }

    /** [localized] es el informe en el idioma de la app; se guarda junto al resultado del motor. */
    fun saveVeriFactuReport(uri: Uri, localized: String) {
        val snapshot = mutableState.value
        if (!snapshot.awaitingReportSave || snapshot.reportKind != ReportKind.VERIFACTU) return
        val report = snapshot.veriFactuReport ?: return cancelReportExport()
        mutableState.value = snapshot.copy(awaitingReportSave = false)
        launchOperation(clearsResult = false) {
            val bytes = VeriFactuText.exportJson(report, localized).encodeToByteArray()
            try {
                repository.write(uri, bytes)
                mutableState.value = mutableState.value.copy(
                    result = OperationResult.Success(UiText.Resource(R.string.result_report_saved)))
            } catch (_: Exception) {
                setError(UiText.Resource(R.string.error_report_export))
            } finally { bytes.fill(0) }
        }
    }

    /** Consulta la API pública de GitHub solo al pulsar «Buscar actualizaciones». */
    fun checkUpdate(currentVersion: String) {
        if (!mutableState.value.canCheckUpdate) return
        mutableState.value = mutableState.value.copy(updateCheck = null)
        launchOperation(clearsResult = false) {
            val check = core.checkUpdate(currentVersion)
            mutableState.value = mutableState.value.copy(updateCheck = check)
        }
    }

    fun dismissUpdateCheck() {
        mutableState.value = mutableState.value.copy(updateCheck = null)
    }

    fun openRelease() {
        val url = mutableState.value.updateCheck?.url?.takeIf(AppLinks::isOfficialRelease) ?: AppLinks.RELEASES
        viewModelScope.launch { effectChannel.send(UiEffect.OpenRelease(url)) }
    }

    private fun runInspect(block: () -> Unit) {
        try {
            block()
        } catch (error: Exception) {
            setError(error.toUserText())
        }
    }

    /**
     * [clearsResult]: una operación nueva vacía la tarjeta de resultado para no
     * mezclarla con la anterior. Guardar lo que ya está en pantalla (la firma,
     * un informe) o las consultas que se muestran en su propio diálogo no la tocan.
     */
    private fun launchOperation(
        allowAwaitingSave: Boolean = false,
        clearsResult: Boolean = !allowAwaitingSave,
        onFinished: () -> Unit = {},
        block: suspend () -> Unit,
    ) {
        if (mutableState.value.busy || mutableState.value.awaitingReportSave || (!allowAwaitingSave && mutableState.value.awaitingSave)) {
            onFinished()
            return
        }
        val current = mutableState.value
        mutableState.value = (if (clearsResult) current.clearedResult() else current).copy(busy = true)
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
        private val settings: AppSettingsStore,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(MainViewModel::class.java))
            return MainViewModel(repository, core, Dispatchers.IO, settings) as T
        }
    }
}

/** Resumen del lote guardado: nombra la carpeta si se conoce, sin caracteres ocultos. */
internal fun batchSavedText(saved: Int, folderName: String?): UiText {
    val name = folderName?.let { io.github.aavidad.grxfirma.android.core.DisplayText.clean(it).trim() }.orEmpty()
    return if (name.isEmpty()) UiText.Plural(R.plurals.batch_saved_count, saved)
    else UiText.Plural(R.plurals.batch_saved_count_named, saved, listOf(name))
}
