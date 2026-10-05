// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android

import android.content.Intent
import android.graphics.BitmapFactory
import android.net.Uri
import android.nfc.NfcAdapter
import android.nfc.Tag
import android.os.Bundle
import android.provider.Settings
import android.text.InputType
import android.view.WindowManager
import android.view.View
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.app.AppCompatDelegate
import androidx.core.os.LocaleListCompat
import androidx.core.widget.doAfterTextChanged
import android.widget.AdapterView
import android.text.util.Linkify
import android.text.method.LinkMovementMethod
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.textfield.TextInputEditText
import com.google.android.material.textfield.TextInputLayout
import es.dipgra.grxfirma.android.core.ReflectiveGomobileBridge
import es.dipgra.grxfirma.android.databinding.ActivityMainBinding
import es.dipgra.grxfirma.android.files.ContentRepository
import es.dipgra.grxfirma.android.intents.IncomingDocument
import es.dipgra.grxfirma.android.intents.IntentDocumentResolver
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.nfc.DnieInput
import es.dipgra.grxfirma.android.nfc.DnieNfcSession
import es.dipgra.grxfirma.android.seal.SealEditorDialog
import es.dipgra.grxfirma.android.seal.SealPreferences
import es.dipgra.grxfirma.android.seal.SealSettings
import es.dipgra.grxfirma.android.files.DocumentPolicy
import es.dipgra.grxfirma.android.ui.MainUiState
import es.dipgra.grxfirma.android.ui.MainViewModel
import es.dipgra.grxfirma.android.ui.OperationResult
import es.dipgra.grxfirma.android.ui.PendingKind
import es.dipgra.grxfirma.android.ui.ToolsPolicy
import es.dipgra.grxfirma.android.ui.UiEffect
import es.dipgra.grxfirma.android.ui.UiText
import es.dipgra.grxfirma.android.ui.resolve
import es.dipgra.grxfirma.android.ui.toUiText
import kotlinx.coroutines.launch
import java.io.File
import java.util.Base64
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

private const val STATE_EXPANDED_TOOLS = "expanded_tools"

class MainActivity : AppCompatActivity() {
    private lateinit var binding: ActivityMainBinding
    private lateinit var sealPreferences: SealPreferences
    private var sealSettings = SealSettings()
    private var sealPageInfo: Triple<Int, Int, Int>? = null
    private var sealEditor: SealEditorDialog? = null
    private var sealEditorOpen = false
    private var updatingSealCheck = false
    private var lastDocumentUri: Uri? = null
    private var updatingSigningControls = false
    private val sealImageFile: File by lazy { File(noBackupFilesDir, "visible-seal-image") }
    private var pendingCan: CharArray? = null
    private var dnieSession: DnieNfcSession? = null
    private val readingDnie = AtomicBoolean(false)
    private val scanEpoch = AtomicLong()
    private val nfcAdapter: NfcAdapter? by lazy { NfcAdapter.getDefaultAdapter(this) }
    private val expandedTools = mutableSetOf<Int>()
    private var updatingToolControls = false

    private val viewModel: MainViewModel by viewModels {
        MainViewModel.Factory(
            ContentRepository(contentResolver),
            ReflectiveGomobileBridge.create(applicationContext),
        )
    }

    private val openDocument = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let {
            viewModel.selectDocument(it)
        }
    }

    private val openOriginalDocument =
        registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
            uri?.let {
                viewModel.selectOriginalDocument(it)
            }
        }

    private val openCertificate = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let {
            viewModel.selectCertificateFile(it)
        }
    }

    private val openSealImage = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) importSealImage(uri)
    }

    private val openBatchDocuments =
        registerForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
            viewModel.selectBatchDocuments(uris)
        }

    private val openHashFile = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(viewModel::checkHash)
    }

    private val openRecipient = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(viewModel::addRecipient)
    }

    private val chooseBatchFolder = registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        if (uri != null) viewModel.saveBatchOutputs(uri) else viewModel.reportSavePickerCancelled()
    }

    private val createSignedDocument = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        val uri = result.data?.data
        if (result.resultCode == RESULT_OK && uri != null) {
            viewModel.savePendingOutput(uri)
        } else {
            viewModel.reportSavePickerCancelled()
        }
    }

    private val createVerificationReport = registerForActivityResult(
        ActivityResultContracts.CreateDocument("application/json"),
    ) { uri ->
        if (uri != null) viewModel.saveVerificationReport(uri) else viewModel.cancelReportExport()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, true)
        if (BuildConfig.CORE_MODE == "production") {
            window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
        binding = ActivityMainBinding.inflate(layoutInflater)
        sealPreferences = SealPreferences(this)
        sealSettings = sealPreferences.load()
        setContentView(binding.root)
        with(binding) {
            ViewCompat.setAccessibilityHeading(documentSectionTitle, true)
            ViewCompat.setAccessibilityHeading(certificateSectionTitle, true)
            ViewCompat.setAccessibilityHeading(operationSectionTitle, true)
            ViewCompat.setAccessibilityHeading(resultSectionTitle, true)
            ViewCompat.setAccessibilityHeading(tools.toolsSectionTitle, true)
        }
        savedInstanceState?.getIntArray(STATE_EXPANDED_TOOLS)?.let { expandedTools.addAll(it.toList()) }
        releaseLegacyPersistedPermissions()
        configureActions()
        collectViewModel()
        if (savedInstanceState == null) handleIncomingIntent(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleIncomingIntent(intent)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putIntArray(STATE_EXPANDED_TOOLS, expandedTools.toIntArray())
    }

    override fun onDestroy() {
        binding.tools.protectKey.text?.clear()
        binding.tools.protectKeyConfirm.text?.clear()
        stopDnieReading()
        dnieSession?.close()
        dnieSession = null
        binding.certificatePassword.text?.clear()
        super.onDestroy()
    }

    override fun onStop() {
        stopDnieReading()
        if (dnieSession != null) {
            dnieSession?.close()
            dnieSession = null
            viewModel.clearDnieIdentity()
        }
        super.onStop()
    }

    private fun configureActions() = with(binding) {
        selectDocumentButton.setOnClickListener {
            try {
                openDocument.launch(arrayOf("*/*"))
            } catch (_: RuntimeException) {
                viewModel.reportPickerError()
            }
        }
        selectOriginalDocumentButton.setOnClickListener {
            try {
                openOriginalDocument.launch(arrayOf("*/*"))
            } catch (_: RuntimeException) {
                viewModel.reportPickerError()
            }
        }
        clearOriginalDocumentButton.setOnClickListener { viewModel.clearOriginalDocument() }
        helpButton.setOnClickListener { showHelp() }
        aboutButton.setOnClickListener { showAbout() }
        languageButton.setOnClickListener { showLanguageSelector() }
        exportReportButton.setOnClickListener { viewModel.exportVerificationReport() }
        useCosignButton.setOnClickListener { viewModel.acceptCoSignSuggestion() }
        val signingListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                updateSigningSettings()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) = Unit
        }
        signatureAction.onItemSelectedListener = signingListener
        signatureProfile.onItemSelectedListener = signingListener
        tsaEnabled.setOnCheckedChangeListener { _, _ -> updateSigningSettings() }
        tsaUrl.doAfterTextChanged { updateSigningSettings() }
        selectCertificateFileButton.setOnClickListener {
            stopDnieReading()
            if (dnieSession != null) {
                dnieSession?.close()
                dnieSession = null
                viewModel.clearDnieIdentity()
            }
            try {
                openCertificate.launch(
                    arrayOf(
                        "application/x-pkcs12",
                        "application/pkcs12",
                        "application/octet-stream",
                    ),
                )
            } catch (_: RuntimeException) {
                viewModel.reportPickerError()
            }
        }
        selectDnieNfcButton.setOnClickListener { startDnieSelection() }
        cancelDnieScanButton.setOnClickListener { stopDnieReading() }
        importCertificateButton.setOnClickListener {
            val editable = certificatePassword.text
            val password = CharArray(editable?.length ?: 0) { editable!![it] }
            certificatePassword.text?.clear()
            certificatePasswordLayout.error = null
            if (password.isEmpty()) {
                password.fill('\u0000')
                certificatePasswordLayout.error = getString(R.string.error_password_required)
            } else {
                viewModel.importCertificate(password)
            }
        }
        forgetCertificateButton.setOnClickListener {
            dnieSession?.close()
            dnieSession = null
            viewModel.forgetCertificate()
        }
        visibleSealCheck.setOnCheckedChangeListener { _, checked ->
            if (updatingSealCheck) return@setOnCheckedChangeListener
            sealSettings = sealSettings.copy(enabled = checked)
            sealPreferences.save(sealSettings)
            editVisibleSealButton.visibility = if (checked) View.VISIBLE else View.GONE
            if (checked && sealPageInfo == null) showSealEditor()
        }
        editVisibleSealButton.setOnClickListener { showSealEditor() }
        signButton.setOnClickListener {
            if (dnieSession != null) {
                val document = viewModel.state.value.document
                if (sealSettings.enabled && document != null && isPdf(document) && sealPageInfo == null) {
                    showSealEditor()
                } else {
                    showDniePinDialog()
                }
            } else signWithSealIfSelected()
        }
        verifyButton.setOnClickListener { viewModel.verify() }
        retrySaveButton.setOnClickListener { viewModel.retryPendingOutput() }
        discardPendingOutputButton.setOnClickListener { viewModel.discardPendingOutput() }
        configureTools()
    }

    private fun configureTools() = with(binding.tools) {
        listOf(toggleBatchButton to batchGroup, toggleHashButton to hashGroup, toggleProtectButton to protectGroup)
            .forEach { (toggle, group) ->
                toggle.setOnClickListener {
                    if (!expandedTools.remove(group.id)) expandedTools += group.id
                    renderToolToggles()
                }
            }
        renderToolToggles()
        val toolListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                updateToolSettings()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) = Unit
        }
        hashAlgorithm.onItemSelectedListener = toolListener
        hashFormat.onItemSelectedListener = toolListener
        protectionContainer.onItemSelectedListener = toolListener
        protectForMe.setOnCheckedChangeListener { _, _ -> updateToolSettings() }
        selectBatchButton.setOnClickListener {
            try { openBatchDocuments.launch(arrayOf("*/*")) } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
        clearBatchButton.setOnClickListener { viewModel.clearBatchDocuments() }
        signBatchButton.setOnClickListener { viewModel.signBatch(selectedSignatureFormat()) }
        createHashButton.setOnClickListener { viewModel.createHash() }
        checkHashButton.setOnClickListener {
            try { openHashFile.launch(arrayOf("*/*")) } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
        addRecipientButton.setOnClickListener {
            try {
                openRecipient.launch(arrayOf("application/pkix-cert", "application/x-x509-ca-cert",
                    "application/x-x509-user-cert", "application/x-pem-file", "application/octet-stream"))
            } catch (_: RuntimeException) {
                viewModel.reportPickerError()
            }
        }
        clearRecipientsButton.setOnClickListener { viewModel.clearRecipients() }
        generateKeyButton.setOnClickListener {
            val key = ToolsPolicy.generateAesKey()
            try {
                protectKey.setText(key, 0, key.size)
                protectKeyConfirm.setText(key, 0, key.size)
            } finally {
                key.fill('\u0000')
            }
            protectKeyLayout.helperText = getString(R.string.protect_key_generated)
        }
        protectButton.setOnClickListener { protectWithKey(sign = false) }
        protectSignButton.setOnClickListener { protectWithKey(sign = true) }
        unprotectButton.setOnClickListener {
            val key = readAndClear(protectKey)
            protectKeyConfirm.text?.clear()
            viewModel.unprotect(key)
        }
    }

    private fun protectWithKey(sign: Boolean) {
        val key = readAndClear(binding.tools.protectKey)
        val confirmation = readAndClear(binding.tools.protectKeyConfirm)
        binding.tools.protectKeyLayout.helperText = getString(R.string.protect_key_helper)
        viewModel.protect(key, confirmation, sign)
    }

    private fun readAndClear(field: TextInputEditText): CharArray {
        val editable = field.text
        val chars = CharArray(editable?.length ?: 0) { editable!![it] }
        field.text?.clear()
        return chars
    }

    private fun renderToolToggles() = with(binding.tools) {
        listOf(toggleBatchButton to batchGroup, toggleHashButton to hashGroup, toggleProtectButton to protectGroup)
            .forEach { (toggle, group) ->
                val expanded = group.id in expandedTools
                group.visibility = if (expanded) View.VISIBLE else View.GONE
                toggle.setIconResource(if (expanded) R.drawable.ic_expand_less else R.drawable.ic_expand_more)
                ViewCompat.setStateDescription(toggle,
                    getString(if (expanded) R.string.state_expanded else R.string.state_collapsed))
            }
    }

    private fun updateToolSettings() {
        if (updatingToolControls) return
        viewModel.updateToolSettings(
            ToolsPolicy.HASH_ALGORITHMS.getOrElse(binding.tools.hashAlgorithm.selectedItemPosition) { "SHA-256" },
            ToolsPolicy.HASH_FORMATS.getOrElse(binding.tools.hashFormat.selectedItemPosition) { "hex" },
            ToolsPolicy.CONTAINERS.getOrElse(binding.tools.protectionContainer.selectedItemPosition) { "cms" },
            binding.tools.protectForMe.isChecked,
        )
    }

    private fun renderTools(state: MainUiState) = with(binding.tools) {
        toolsHelper.setText(if (state.backend.available && state.toolsAvailable) R.string.tools_helper else R.string.tools_unavailable)
        val idle = state.canReplaceSelection
        listOf(toggleBatchButton, toggleHashButton, toggleProtectButton).forEach { it.isEnabled = true }
        selectBatchButton.isEnabled = state.canUseTools
        batchSummary.text = if (state.batchDocuments.isEmpty()) getString(R.string.batch_none) else
            resources.getQuantityString(R.plurals.batch_selected, state.batchDocuments.size, state.batchDocuments.size) +
                "\n" + state.batchDocuments.joinToString("\n") { it.summaryText() }
        clearBatchButton.visibility = if (state.batchDocuments.isEmpty()) View.GONE else View.VISIBLE
        clearBatchButton.isEnabled = idle
        signBatchButton.isEnabled = state.canSignBatch
        hashAlgorithm.isEnabled = idle
        hashFormat.isEnabled = idle
        createHashButton.isEnabled = state.canHash
        checkHashButton.isEnabled = state.canHash
        protectionContainer.isEnabled = idle
        val transient = state.usesTransientKey
        protectForMe.visibility = if (transient) View.GONE else View.VISIBLE
        protectForMe.isEnabled = idle && state.certificate != null && !state.certificateExternal
        addRecipientButton.visibility = protectForMe.visibility
        addRecipientButton.isEnabled = idle && state.recipients.size < ToolsPolicy.MAX_RECIPIENTS
        recipientsSummary.visibility = protectForMe.visibility
        recipientsSummary.text = if (state.recipients.isEmpty()) getString(R.string.protect_recipients_none) else
            resources.getQuantityString(R.plurals.protect_recipients_count, state.recipients.size, state.recipients.size) +
                "\n" + state.recipients.joinToString("\n") { it.displayName }
        clearRecipientsButton.visibility = if (!transient && state.recipients.isNotEmpty()) View.VISIBLE else View.GONE
        clearRecipientsButton.isEnabled = idle
        val encryptedInput = state.document?.displayName?.endsWith(".encrypted.p7m", ignoreCase = true) == true
        protectKeyLayout.visibility = if (transient || encryptedInput) View.VISIBLE else View.GONE
        protectKeyConfirmLayout.visibility = if (transient) View.VISIBLE else View.GONE
        generateKeyButton.visibility = protectKeyConfirmLayout.visibility
        protectKeyLayout.isEnabled = !state.busy
        protectKeyConfirmLayout.isEnabled = !state.busy
        generateKeyButton.isEnabled = idle
        protectButton.isEnabled = state.canProtect
        protectSignButton.visibility = if (transient) View.GONE else View.VISIBLE
        protectSignButton.isEnabled = state.canProtectAndSign
        unprotectButton.isEnabled = state.canUnprotect
        binding.discardPendingOutputButton.setText(
            if (state.pendingKind == PendingKind.SIGNATURE) R.string.discard_pending_output else R.string.discard_pending_tool_output,
        )
        updatingToolControls = true
        hashAlgorithm.setSelection(ToolsPolicy.HASH_ALGORITHMS.indexOf(state.hashAlgorithm).coerceAtLeast(0))
        hashFormat.setSelection(ToolsPolicy.HASH_FORMATS.indexOf(state.hashFormat).coerceAtLeast(0))
        protectionContainer.setSelection(ToolsPolicy.CONTAINERS.indexOf(state.protectionContainer).coerceAtLeast(0))
        protectForMe.isChecked = state.protectForMe
        updatingToolControls = false
    }

    private fun collectViewModel() {
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                launch { viewModel.state.collect(::render) }
                launch {
                    viewModel.effects.collect { effect ->
                        when (effect) {
                            is UiEffect.SaveSignedDocument -> requestSave(effect)
                            UiEffect.ChooseBatchFolder -> try {
                                chooseBatchFolder.launch(null)
                            } catch (_: RuntimeException) { viewModel.reportSavePickerUnavailable() }
                            UiEffect.SaveVerificationReport -> try {
                                createVerificationReport.launch(getString(R.string.verification_report_filename))
                            } catch (_: RuntimeException) { viewModel.cancelReportExport() }
                        }
                    }
                }
            }
        }
    }

    private fun render(state: MainUiState) = with(binding) {
        if (lastDocumentUri != state.document?.uri) {
            lastDocumentUri = state.document?.uri
            sealPageInfo = null
        }
        backendStatusTitle.setText(
            if (state.backend.available) {
                R.string.backend_ready_title
            } else {
                R.string.backend_unavailable_title
            },
        )
        backendStatusDetail.setText(
            when {
                state.backend.available -> R.string.backend_ready_detail
                state.backend.code == "verification_build" -> R.string.backend_verification_detail
                else -> R.string.backend_unavailable_detail
            },
        )
        val statusColor = if (state.backend.available) {
            ContextCompat.getColor(this@MainActivity, R.color.primary_container)
        } else {
            ContextCompat.getColor(this@MainActivity, R.color.status_warning_container)
        }
        backendStatusCard.setCardBackgroundColor(statusColor)

        documentSummary.text = state.document?.summaryText() ?: getString(R.string.no_document)
        originalDocumentSummary.text = state.originalDocument?.summaryText()
            ?: getString(R.string.no_original_document)
        certificateFileSummary.text = state.certificateFile?.summaryText()
            ?: getString(R.string.no_certificate_file)
        certificateSummary.text = state.certificate?.let { certificate ->
            getString(
                R.string.certificate_summary,
                certificate.subject,
                certificate.issuer.ifBlank { getString(R.string.unknown_value) },
                certificate.fingerprint.ifBlank { getString(R.string.unknown_value) },
            )
        } ?: getString(R.string.no_certificate)

        selectDocumentButton.isEnabled = state.canReplaceSelection
        selectOriginalDocumentButton.isEnabled = state.canReplaceSelection
        clearOriginalDocumentButton.isEnabled = state.canReplaceSelection
        clearOriginalDocumentButton.visibility =
            if (state.originalDocument == null) View.GONE else View.VISIBLE
        selectCertificateFileButton.isEnabled = state.canReplaceSelection
        selectDnieNfcButton.isEnabled = state.canReplaceSelection && state.backend.available
        importCertificateButton.isEnabled = state.canImportCertificate
        val certificateImportVisibility = if (state.certificateFile != null) View.VISIBLE else View.GONE
        certificatePasswordLayout.visibility = certificateImportVisibility
        importCertificateButton.visibility = certificateImportVisibility
        forgetCertificateButton.isEnabled = state.canForgetCertificate
        forgetCertificateButton.visibility =
            if (state.certificate == null) View.GONE else View.VISIBLE
        signButton.isEnabled = state.canSign
        val pdfSelected = state.document?.let(::isPdf) == true
        visibleSealCheck.visibility = if (pdfSelected) View.VISIBLE else View.GONE
        visibleSealCheck.isEnabled = state.canReplaceSelection
        if (visibleSealCheck.isChecked != sealSettings.enabled) {
            updatingSealCheck = true
            visibleSealCheck.isChecked = sealSettings.enabled
            updatingSealCheck = false
        }
        editVisibleSealButton.visibility = if (pdfSelected && sealSettings.enabled) View.VISIBLE else View.GONE
        editVisibleSealButton.isEnabled = state.canReplaceSelection
        verificationDetail.setTextColor(ContextCompat.getColor(this@MainActivity, when {
            state.postSignVerificationFailed -> R.color.status_warning
            state.verification?.integrityStatus == "invalid" -> R.color.error
            state.verification?.toUiText()?.accredited() == true -> R.color.primary
            else -> R.color.status_warning
        }))
        verifyButton.isEnabled = state.canVerify
        certificatePasswordLayout.isEnabled = !state.busy
        signatureFormat.isEnabled = state.canReplaceSelection
        signatureAction.isEnabled = state.canReplaceSelection
        signatureProfile.isEnabled = state.canReplaceSelection
        tsaEnabled.isEnabled = state.canReplaceSelection
        tsaUrl.isEnabled = state.canReplaceSelection && state.tsaEnabled
        languageButton.isEnabled = state.canReplaceSelection
        aboutButton.isEnabled = state.canReplaceSelection
        helpButton.isEnabled = state.canReplaceSelection
        updatingSigningControls = true
        signatureAction.setSelection(listOf("sign", "cosign", "countersign").indexOf(state.signatureAction).coerceAtLeast(0))
        signatureProfile.setSelection(listOf("baseline", "t", "lt", "lta").indexOf(state.signatureProfile).coerceAtLeast(0))
        tsaEnabled.isChecked = state.tsaEnabled
        if (tsaUrl.text.toString() != state.tsaUrl) tsaUrl.setText(state.tsaUrl)
        updatingSigningControls = false
        useCosignButton.visibility = if (state.coSignSuggested) View.VISIBLE else View.GONE
        useCosignButton.isEnabled = state.canReplaceSelection
        verificationDetail.text = if (state.verification != null || state.postSignVerificationFailed) {
            getString(R.string.verification_document, state.verifiedDocumentName) + "\n" +
                (state.verification?.toUiText()?.resolve(this@MainActivity)
                    ?: getString(R.string.post_sign_verification_failed))
        } else ""
        verificationDetail.visibility = if (state.verification != null || state.postSignVerificationFailed) View.VISIBLE else View.GONE
        exportReportButton.visibility = if (state.verification?.reportJson?.isNotEmpty() == true) View.VISIBLE else View.GONE
        exportReportButton.isEnabled = state.canExportReport
        progressContainer.visibility = if (state.busy) View.VISIBLE else View.GONE
        pendingSaveActions.visibility = if (state.canRetryPendingOutput) View.VISIBLE else View.GONE
        retrySaveButton.isEnabled = state.canRetryPendingOutput
        discardPendingOutputButton.isEnabled = state.canDiscardPendingOutput
        renderTools(state)

        when (val result = state.result) {
            OperationResult.Idle -> {
                resultTitle.setText(R.string.result_idle)
                resultTitle.setTextColor(
                    ContextCompat.getColor(this@MainActivity, R.color.on_surface),
                )
                resultDetail.visibility = View.GONE
            }
            is OperationResult.Success -> {
                resultTitle.text = result.title.resolve(this@MainActivity)
                val verification = result.detail as? UiText.Verification
                val color = when {
                    verification == null -> R.color.primary
                    !verification.valid || verification.integrityStatus == "invalid" -> R.color.error
                    verification.accredited() -> R.color.primary
                    else -> R.color.status_warning
                }
                resultTitle.setTextColor(ContextCompat.getColor(this@MainActivity, color))
                renderDetail(if (result.detail is UiText.Verification) null else result.detail?.resolve(this@MainActivity))
            }
            is OperationResult.Error -> {
                dnieSession?.consumeSigningError()?.let { error ->
                    viewModel.reportDnieError(error, dnieSession?.retriesLeft() ?: -1)
                    return@with
                }
                dnieSession?.clearPin()
                resultTitle.setText(R.string.result_error)
                resultTitle.setTextColor(ContextCompat.getColor(this@MainActivity, R.color.error))
                renderDetail(result.detail.resolve(this@MainActivity))
            }
        }
    }

    private fun renderDetail(detail: String?) = with(binding.resultDetail) {
        text = detail.orEmpty()
        visibility = if (detail.isNullOrBlank()) View.GONE else View.VISIBLE
    }

    private fun SelectedFile.summaryText(): String = getString(
        R.string.document_summary,
        displayName,
        mimeType,
        formatSize(sizeBytes),
    )

    private fun formatSize(bytes: Long?): String = when {
        bytes == null -> getString(R.string.unknown_value)
        bytes < 1024 -> getString(R.string.size_bytes, bytes)
        bytes < 1024 * 1024 -> getString(R.string.size_kib, bytes / 1024.0)
        else -> getString(R.string.size_mib, bytes / (1024.0 * 1024.0))
    }

    private fun updateSigningSettings() {
        if (updatingSigningControls) return
        viewModel.updateSigningSettings(
            listOf("sign", "cosign", "countersign").getOrElse(binding.signatureAction.selectedItemPosition) { "sign" },
            listOf("baseline", "t", "lt", "lta").getOrElse(binding.signatureProfile.selectedItemPosition) { "baseline" },
            binding.tsaEnabled.isChecked, binding.tsaUrl.text?.toString().orEmpty(),
        )
    }

    private fun showLanguageSelector() {
        val tags = resources.getStringArray(R.array.language_tags)
        val current = AppCompatDelegate.getApplicationLocales().toLanguageTags()
        MaterialAlertDialogBuilder(this)
            .setTitle(R.string.language_title)
            .setSingleChoiceItems(R.array.language_names, tags.indexOf(current).coerceAtLeast(0)) { dialog, index ->
                dialog.dismiss()
                AppCompatDelegate.setApplicationLocales(LocaleListCompat.forLanguageTags(tags[index]))
            }
            .setNegativeButton(R.string.help_close, null)
            .show()
    }

    private fun showAbout() {
        val engine = viewModel.engineVersion.takeIf { it.isNotBlank() && it != "development" }
            ?: getString(R.string.unknown_value)
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.about_title)
            .setMessage(getString(R.string.about_content, BuildConfig.VERSION_NAME, engine))
            .setPositiveButton(R.string.help_close, null)
            .setNeutralButton(R.string.about_release_notes) { _, _ ->
                MaterialAlertDialogBuilder(this).setTitle(R.string.about_release_notes)
                    .setMessage(R.string.release_notes_content).setPositiveButton(R.string.help_close, null).show()
            }
            .setNegativeButton(R.string.open_help) { _, _ -> showHelp() }
            .show()
        dialog.findViewById<android.widget.TextView>(android.R.id.message)?.let {
            Linkify.addLinks(it, Linkify.EMAIL_ADDRESSES or Linkify.WEB_URLS)
            it.movementMethod = LinkMovementMethod.getInstance()
        }
    }

    private fun selectedSignatureFormat(): String = when (binding.signatureFormat.selectedItemPosition) {
        1 -> "pades"
        2 -> "cades"
        3 -> "xades"
        else -> "auto"
    }

    private fun isPdf(file: SelectedFile): Boolean =
        file.mimeType.equals("application/pdf", ignoreCase = true) || file.displayName.endsWith(".pdf", ignoreCase = true)

    private fun showSealEditor() {
        if (sealEditorOpen) return
        val document = viewModel.state.value.document
        if (document == null || !isPdf(document)) return
        if (viewModel.state.value.certificate == null) {
            android.widget.Toast.makeText(this, R.string.seal_certificate_required, android.widget.Toast.LENGTH_LONG).show()
            return
        }
        sealEditorOpen = true
        sealEditor = SealEditorDialog(this, viewModel, document, sealSettings, sealImageFile,
            chooseImage = {
                try { openSealImage.launch(arrayOf("image/png", "image/jpeg")) }
                catch (_: RuntimeException) {
                    android.widget.Toast.makeText(this, R.string.error_picker_unavailable, android.widget.Toast.LENGTH_LONG).show()
                }
            },
            onSaved = { settings, count, width, height ->
                sealSettings = settings
                sealPageInfo = Triple(count, width, height)
                sealPreferences.save(settings)
            },
            onClosed = {
                sealEditorOpen = false
                sealEditor = null
            })
        sealEditor?.show()
    }

    private fun signWithSealIfSelected() {
        val format = selectedSignatureFormat()
        val document = viewModel.state.value.document
        if (!sealSettings.enabled || document == null || !isPdf(document)) {
            viewModel.sign(format)
            return
        }
        if (format != "auto" && format != "pades") {
            dnieSession?.clearPin()
            android.widget.Toast.makeText(this, R.string.seal_pdf_only, android.widget.Toast.LENGTH_LONG).show()
            return
        }
        val pageInfo = sealPageInfo
        if (pageInfo == null) {
            dnieSession?.clearPin()
            showSealEditor()
            return
        }
        try {
            val image = if (sealSettings.logo == "custom" && sealImageFile.isFile && sealImageFile.length() in 1..(2L shl 20)) {
                val bytes = sealImageFile.readBytes()
                try { Base64.getEncoder().encodeToString(bytes) } finally { bytes.fill(0) }
            } else null
            val options = sealSettings.options(pageInfo.first, pageInfo.second, pageInfo.third, image)
            viewModel.sign("pades", options)
        } catch (_: Exception) {
            dnieSession?.clearPin()
            android.widget.Toast.makeText(this, R.string.seal_invalid_settings, android.widget.Toast.LENGTH_LONG).show()
        }
    }

    private fun importSealImage(uri: Uri) {
        try {
            val bytes = contentResolver.openInputStream(uri)?.use {
                DocumentPolicy.readBounded(it, 2 * 1024 * 1024, getString(R.string.seal_image))
            } ?: throw IllegalArgumentException()
            try {
                val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
                require(bounds.outWidth in 1..8192 && bounds.outHeight in 1..8192 &&
                    bounds.outWidth.toLong() * bounds.outHeight <= 16L * 1024 * 1024)
                sealImageFile.outputStream().use { it.write(bytes) }
            } finally { bytes.fill(0) }
            sealEditor?.customImageSelected()
        } catch (_: Exception) {
            android.widget.Toast.makeText(this, R.string.seal_image_error, android.widget.Toast.LENGTH_LONG).show()
        }
    }

    private fun requestSave(effect: UiEffect.SaveSignedDocument) {
        val intent = Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
            addCategory(Intent.CATEGORY_OPENABLE)
            type = effect.mimeType
            putExtra(Intent.EXTRA_TITLE, effect.displayName)
        }
        try {
            createSignedDocument.launch(intent)
        } catch (_: RuntimeException) {
            viewModel.reportSavePickerUnavailable()
        }
    }

    private fun handleIncomingIntent(intent: Intent?) {
        when (val incoming = IntentDocumentResolver.resolve(intent)) {
            is IncomingDocument.Single -> {
                if (viewModel.canAcceptIncomingDocument()) {
                    viewModel.selectDocument(incoming.uri)
                } else {
                    viewModel.reportBusyIncomingIntent()
                }
            }
            IncomingDocument.Multiple -> {
                if (viewModel.canAcceptIncomingDocument()) {
                    viewModel.reportSharedIntentError(multiple = true)
                } else {
                    viewModel.reportBusyIncomingIntent()
                }
            }
            IncomingDocument.Invalid -> {
                if (viewModel.canAcceptIncomingDocument()) {
                    viewModel.reportSharedIntentError()
                } else {
                    viewModel.reportBusyIncomingIntent()
                }
            }
            IncomingDocument.None -> Unit
        }
    }

    private fun releaseLegacyPersistedPermissions() {
        contentResolver.persistedUriPermissions.forEach { permission ->
            var flags = 0
            if (permission.isReadPermission) flags = flags or Intent.FLAG_GRANT_READ_URI_PERMISSION
            if (permission.isWritePermission) flags = flags or Intent.FLAG_GRANT_WRITE_URI_PERMISSION
            if (flags != 0) {
                try {
                    contentResolver.releasePersistableUriPermission(permission.uri, flags)
                } catch (_: SecurityException) {
                    // El proveedor puede haber revocado ya el permiso.
                }
            }
        }
    }

    private fun startDnieSelection() {
        if (viewModel.state.value.document == null) {
            android.widget.Toast.makeText(this, R.string.dnie_document_first, android.widget.Toast.LENGTH_LONG).show()
            return
        }
        val adapter = nfcAdapter
        if (adapter == null) {
            viewModel.reportDnieError(IllegalStateException("NFC_MISSING"))
            return
        }
        if (!adapter.isEnabled) {
            viewModel.reportDnieError(IllegalStateException("NFC_OFF"))
            MaterialAlertDialogBuilder(this)
                .setMessage(R.string.dnie_nfc_off)
                .setPositiveButton(R.string.dnie_open_nfc_settings) { _, _ ->
                    startActivity(Intent(Settings.ACTION_NFC_SETTINGS))
                }
                .setNegativeButton(R.string.dnie_cancel, null)
                .show()
            return
        }
        val field = TextInputEditText(this).apply {
            inputType = InputType.TYPE_CLASS_NUMBER
            filters = arrayOf(android.text.InputFilter.LengthFilter(6))
            isSaveEnabled = false
            importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_NO_EXCLUDE_DESCENDANTS
        }
        val layout = TextInputLayout(this).apply {
            hint = getString(R.string.dnie_can_label)
            addView(field)
        }
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.dnie_can_title)
            .setMessage(R.string.dnie_can_help)
            .setView(layout)
            .setPositiveButton(R.string.dnie_can_continue, null)
            .setNegativeButton(R.string.dnie_cancel) { _, _ -> field.text?.clear() }
            .create()
        dialog.setOnShowListener {
            dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                val editable = field.text
                val can = CharArray(editable?.length ?: 0) { editable!![it] }
                field.text?.clear()
                if (!DnieInput.validCan(can)) {
                    can.fill('\u0000')
                    layout.error = getString(R.string.dnie_can_invalid)
                    return@setOnClickListener
                }
                pendingCan?.fill('\u0000')
                pendingCan = can
                layout.error = null
                dialog.dismiss()
                beginDnieReading(adapter)
            }
        }
        dialog.show()
    }

    private fun beginDnieReading(adapter: NfcAdapter) {
        val epoch = scanEpoch.incrementAndGet()
        binding.dnieScanStatus.visibility = View.VISIBLE
        binding.cancelDnieScanButton.visibility = View.VISIBLE
        binding.dnieScanStatus.setText(R.string.dnie_scan_instruction)
        readingDnie.set(false)
        adapter.enableReaderMode(this, reader@{ tag: Tag ->
            if (!readingDnie.compareAndSet(false, true)) return@reader
            val can = pendingCan ?: run {
                readingDnie.set(false)
                return@reader
            }
            pendingCan = null
            try {
                val session = DnieNfcSession.open(tag, can)
                runOnUiThread ui@{
                    if (scanEpoch.get() != epoch || !lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) {
                        session.close()
                        return@ui
                    }
                    nfcAdapter?.disableReaderMode(this)
                    binding.cancelDnieScanButton.visibility = View.GONE
                    dnieSession?.close()
                    dnieSession = session
                    viewModel.installDnie(session) {
                        binding.dnieScanStatus.setText(R.string.dnie_ready)
                    }
                }
            } catch (error: Exception) {
                runOnUiThread ui@{
                    if (scanEpoch.get() != epoch || !lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) return@ui
                    stopDnieReading()
                    binding.dnieScanStatus.visibility = View.GONE
                    viewModel.reportDnieError(error)
                }
            } finally {
                can.fill('\u0000')
                readingDnie.set(false)
            }
        }, NfcAdapter.FLAG_READER_NFC_A or NfcAdapter.FLAG_READER_NFC_B or
            NfcAdapter.FLAG_READER_SKIP_NDEF_CHECK, null)
    }

    private fun stopDnieReading() {
        scanEpoch.incrementAndGet()
        try { nfcAdapter?.disableReaderMode(this) } catch (_: IllegalStateException) { }
        pendingCan?.fill('\u0000')
        pendingCan = null
        binding.dnieScanStatus.visibility = View.GONE
        binding.cancelDnieScanButton.visibility = View.GONE
    }

    private fun showDniePinDialog() {
        val session = dnieSession ?: return
        val field = TextInputEditText(this).apply {
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
            filters = arrayOf(android.text.InputFilter.LengthFilter(16))
            isSaveEnabled = false
            importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_NO_EXCLUDE_DESCENDANTS
        }
        val layout = TextInputLayout(this).apply {
            hint = getString(R.string.dnie_pin_label)
            addView(field)
        }
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.dnie_pin_title)
            .setMessage(R.string.dnie_pin_help)
            .setView(layout)
            .setPositiveButton(R.string.dnie_pin_confirm, null)
            .setNegativeButton(R.string.dnie_cancel) { _, _ -> field.text?.clear() }
            .create()
        dialog.setOnDismissListener {
            field.text?.clear()
            if (BuildConfig.CORE_MODE != "production") window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
        dialog.setOnShowListener {
            dialog.window?.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
            dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                val editable = field.text
                val pin = CharArray(editable?.length ?: 0) { editable!![it] }
                field.text?.clear()
                if (!DnieInput.validPin(pin)) {
                    pin.fill('\u0000')
                    layout.error = getString(R.string.dnie_pin_invalid)
                    return@setOnClickListener
                }
                try {
                    session.setPin(pin)
                    dialog.dismiss()
                    signWithSealIfSelected()
                } finally {
                    pin.fill('\u0000')
                }
            }
        }
        dialog.show()
    }

    private fun showHelp() {
        MaterialAlertDialogBuilder(this)
            .setTitle(R.string.help_title)
            .setMessage(getString(R.string.help_content) + "\n\n" + getString(R.string.help_tools_content))
            .setPositiveButton(R.string.help_close, null)
            .show()
    }
}
