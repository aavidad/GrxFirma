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
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContract
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.app.ActivityOptionsCompat
import androidx.activity.viewModels
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.app.AppCompatDelegate
import androidx.core.os.LocaleListCompat
import androidx.core.widget.doAfterTextChanged
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
import es.dipgra.grxfirma.android.ui.AutoClose
import es.dipgra.grxfirma.android.ui.MainUiState
import es.dipgra.grxfirma.android.ui.MainViewModel
import es.dipgra.grxfirma.android.ui.OperationResult
import es.dipgra.grxfirma.android.ui.PendingKind
import es.dipgra.grxfirma.android.ui.ToolsPolicy
import es.dipgra.grxfirma.android.ui.FormatPolicy
import es.dipgra.grxfirma.android.ui.EniForm
import es.dipgra.grxfirma.android.ui.EngineText
import es.dipgra.grxfirma.android.model.EniRequest
import com.google.android.material.datepicker.CalendarConstraints
import com.google.android.material.datepicker.DateValidatorPointBackward
import com.google.android.material.datepicker.MaterialDatePicker
import java.text.DateFormat
import java.util.Date
import java.util.TimeZone
import es.dipgra.grxfirma.android.ui.UiEffect
import es.dipgra.grxfirma.android.ui.AppFacts
import es.dipgra.grxfirma.android.ui.CertificateFilter
import es.dipgra.grxfirma.android.ui.CertificateText
import es.dipgra.grxfirma.android.ui.DiagnosticsText
import es.dipgra.grxfirma.android.ui.QrText
import es.dipgra.grxfirma.android.ui.UpdateText
import es.dipgra.grxfirma.android.ui.LanguageTags
import es.dipgra.grxfirma.android.ui.PlatformServicesVisibility
import androidx.core.net.toUri
import es.dipgra.grxfirma.android.ui.VeriFactuText
import es.dipgra.grxfirma.android.core.AppLinks
import es.dipgra.grxfirma.android.databinding.DialogDiagnosticsBinding
import es.dipgra.grxfirma.android.databinding.DialogPreferencesBinding
import es.dipgra.grxfirma.android.model.UpdateCheck
import es.dipgra.grxfirma.android.settings.AppPreferences
import es.dipgra.grxfirma.android.settings.AppSettings
import es.dipgra.grxfirma.android.settings.OutputNames
import es.dipgra.grxfirma.android.ui.Wave4Screen
import es.dipgra.grxfirma.android.ui.eniFileAvailable
import es.dipgra.grxfirma.android.ui.externalBatchAvailable
import es.dipgra.grxfirma.android.ui.batchSealAvailable
import es.dipgra.grxfirma.android.seal.PdfBatchSealPlanner
import es.dipgra.grxfirma.android.seal.BatchSeal
import es.dipgra.grxfirma.android.ui.UiText
import es.dipgra.grxfirma.android.ui.resolve
import es.dipgra.grxfirma.android.ui.toUiText
import kotlinx.coroutines.launch
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.view.Menu
import android.view.MenuItem
import com.google.android.material.snackbar.Snackbar
import java.io.File
import java.util.Base64
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import android.content.ActivityNotFoundException
import android.content.pm.PackageManager
import es.dipgra.grxfirma.android.qr.QrCapture
import es.dipgra.grxfirma.android.qr.QrImagePreparer
import es.dipgra.grxfirma.android.ui.IdentityPanel
import es.dipgra.grxfirma.android.ui.VerificationCard
import es.dipgra.grxfirma.android.model.VerificationSummary
import android.graphics.Typeface
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.style.ForegroundColorSpan
import android.text.style.StyleSpan

private const val STATE_EXPANDED_TOOLS = "expanded_tools"
private const val ENI_DATE_PICKER = "eni_capture_date"
private const val STATE_EXPANDED_EXPEDIENTE = "expanded_expediente"
/** Margen antes de empezar a contar el cierre automático si se sale con un selector del sistema abierto. */
private const val PICKER_GRACE_MS = 15 * 60_000L
/** Nueva comprobación del cierre automático si al vencer había una operación o un guardado pendiente. */
private const val CLOSE_RETRY_MS = 30_000L

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
    private var formatMenu: List<String> = emptyList()
    private var eniStates: List<String> = emptyList()
    private var eniTypes: List<String> = emptyList()
    private var diagnosticsDialog: DialogDiagnosticsBinding? = null
    private var shownUpdateCheck: UpdateCheck? = null
    private var updatingCertificateFilter = false
    private var menuEnabled = true
    private var lastResult: OperationResult? = null
    private var lastVerification: VerificationSummary? = null
    private var verificationTechnicalExpanded = false
    /** Momento (reloj `elapsedRealtime`) desde el que cuenta el cierre automático; 0 en primer plano. */
    private var backgroundSince = 0L
    /** Hay un selector o diálogo de guardado del sistema abierto por la app. */
    private var systemPickerOpen = false
    private val closeHandler = Handler(Looper.getMainLooper())
    private val closeCertificateTask = Runnable {
        when (closeCertificateIfInactive()) {
            AutoClose.NOT_DUE -> scheduleCloseCheck()
            AutoClose.POSTPONED -> scheduleCloseCheck(retry = true)
            else -> Unit
        }
    }
    private lateinit var wave4: Wave4Screen

    private val viewModel: MainViewModel by viewModels {
        MainViewModel.Factory(
            ContentRepository(contentResolver),
            ReflectiveGomobileBridge.create(applicationContext),
            AppPreferences(applicationContext),
        )
    }

    private val openDocument = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let {
            viewModel.selectDocument(it)
        }
    }

    private val openOriginalDocument =
        registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
            uri?.let {
                viewModel.selectOriginalDocument(it)
            }
        }

    private val openCertificate = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let {
            viewModel.selectCertificateFile(it)
        }
    }

    // Foto del QR: la cámara del sistema escribe en un fichero temporal privado
    // (sin permiso CAMERA) que se borra al terminar la lectura o al cancelar.
    private val takeQrPhoto = registerPicker(ActivityResultContracts.TakePicture()) { saved ->
        val photo = QrCapture.file(this)
        if (saved && photo.isFile && photo.length() > 0) {
            viewModel.readVeriFactuQrImage(
                androidx.core.content.FileProvider.getUriForFile(this, QrCapture.authority(this), photo),
                QrImagePreparer::prepare,
            ) { QrCapture.clear(applicationContext) }
        } else {
            QrCapture.clear(this)
        }
    }

    private val openQrImage = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) viewModel.readVeriFactuQrImage(uri, QrImagePreparer::prepare)
    }

    private val openSealImage = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) importSealImage(uri)
    }

    private val openBatchDocuments =
        registerPicker(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
            viewModel.selectBatchDocuments(uris)
        }

    private val openHashFile = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(viewModel::checkHash)
    }

    private val openRecipient = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(viewModel::addRecipient)
    }

    private val openVeriFactuRecords =
        registerPicker(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
            viewModel.selectVeriFactuRecords(uris)
        }

    private val openEniDocument = registerPicker(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(viewModel::validateEni)
    }

    private val chooseBatchFolder = registerPicker(ActivityResultContracts.OpenDocumentTree()) { uri ->
        if (uri != null) viewModel.saveBatchOutputs(uri) else viewModel.reportSavePickerCancelled()
    }

    private val createSignedDocument = registerPicker(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        val uri = result.data?.data
        if (result.resultCode == RESULT_OK && uri != null) {
            viewModel.savePendingOutput(uri)
        } else {
            viewModel.reportSavePickerCancelled()
        }
    }

    private val createVeriFactuReport = registerPicker(
        ActivityResultContracts.CreateDocument("application/json"),
    ) { uri ->
        val report = viewModel.state.value.veriFactuReport
        if (uri != null && report != null) {
            viewModel.saveVeriFactuReport(uri, VeriFactuText.lines(report).resolve(this))
        } else viewModel.cancelReportExport()
    }

    private val createVerificationHtml = registerPicker(
        ActivityResultContracts.CreateDocument("text/html"),
    ) { uri ->
        if (uri != null) viewModel.saveVerificationReport(uri) else viewModel.cancelReportExport()
    }

    private val createVerificationReport = registerPicker(
        ActivityResultContracts.CreateDocument("application/json"),
    ) { uri ->
        if (uri != null) viewModel.saveVerificationReport(uri) else viewModel.cancelReportExport()
    }

    /**
     * Registra un selector del sistema y anota cuándo está abierto: mientras la
     * persona busca una carpeta o un fichero no cuenta el cierre automático.
     */
    private fun <I, O> registerPicker(contract: ActivityResultContract<I, O>, callback: (O) -> Unit): ActivityResultLauncher<I> {
        val launcher = registerForActivityResult(contract) { result ->
            systemPickerOpen = false
            callback(result)
        }
        return object : ActivityResultLauncher<I>() {
            override val contract: ActivityResultContract<I, *> get() = launcher.contract
            override fun launch(input: I, options: ActivityOptionsCompat?) {
                systemPickerOpen = true
                try {
                    launcher.launch(input, options)
                } catch (error: RuntimeException) {
                    systemPickerOpen = false
                    throw error
                }
            }
            override fun unregister() = launcher.unregister()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        AppPreferences.applyTheme(this)
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, true)
        if (BuildConfig.CORE_MODE == "production") {
            window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
        binding = ActivityMainBinding.inflate(layoutInflater)
        sealPreferences = SealPreferences(this)
        sealSettings = sealPreferences.load()
        setContentView(binding.root)
        setSupportActionBar(binding.toolbar)
        with(binding) {
            ViewCompat.setAccessibilityHeading(documentSectionTitle, true)
            ViewCompat.setAccessibilityHeading(certificateSectionTitle, true)
            ViewCompat.setAccessibilityHeading(operationSectionTitle, true)
            ViewCompat.setAccessibilityHeading(resultSectionTitle, true)
            ViewCompat.setAccessibilityHeading(documents.documentsSectionTitle, true)
        }
        savedInstanceState?.getIntArray(STATE_EXPANDED_TOOLS)?.let { expandedTools.addAll(it.toList()) }
        wave4 = Wave4Screen(this, viewModel, dnieAccess)
        wave4.bind(binding.documents.expediente, binding.tools.batchWave4,
            savedInstanceState?.getBoolean(STATE_EXPANDED_EXPEDIENTE) == true)
        releaseLegacyPersistedPermissions()
        // Copias temporales del lote que un cierre inesperado pudo dejar. Al girar
        // la pantalla con un lote en curso no se tocan: el lote aún las usa.
        if (!viewModel.state.value.busy) {
            BatchSeal.clearWorkDirectory(File(noBackupFilesDir, BatchSeal.WORK_DIRECTORY))
            // Una foto del QR que un cierre inesperado pudo dejar.
            QrCapture.clear(this)
        }
        configureActions()
        // El formato por defecto solo se aplica al abrir; al girar se conserva el elegido.
        if (savedInstanceState == null) selectDefaultFormat()
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
        if (::wave4.isInitialized) outState.putBoolean(STATE_EXPANDED_EXPEDIENTE, wave4.isExpanded)
    }

    override fun onDestroy() {
        binding.tools.protectKey.text?.clear()
        binding.tools.protectKeyConfirm.text?.clear()
        stopDnieReading()
        dnieSession?.close()
        dnieSession = null
        binding.certificatePassword.text?.clear()
        closeHandler.removeCallbacks(closeCertificateTask)
        super.onDestroy()
    }

    override fun onStart() {
        super.onStart()
        closeHandler.removeCallbacks(closeCertificateTask)
        closeCertificateIfInactive()
        backgroundSince = 0L
    }

    /** Cierra el certificado si la app lleva en segundo plano más del tiempo elegido. */
    private fun closeCertificateIfInactive(): AutoClose {
        if (backgroundSince == 0L) return AutoClose.NOT_APPLICABLE
        return viewModel.closeCertificateAfterBackground(SystemClock.elapsedRealtime() - backgroundSince)
    }

    /**
     * Programa la siguiente comprobación con el mismo reloj monótono que mide
     * el plazo. El temporizador del sistema puede retrasarse con el móvil en
     * reposo; por eso también se comprueba al volver a la app.
     */
    private fun scheduleCloseCheck(retry: Boolean = false) {
        closeHandler.removeCallbacks(closeCertificateTask)
        val minutes = viewModel.state.value.settings.sessionTimeoutMinutes
        if (backgroundSince == 0L || minutes <= 0) return
        val remaining = backgroundSince + minutes * 60_000L - SystemClock.elapsedRealtime()
        closeHandler.postDelayed(closeCertificateTask, if (retry) CLOSE_RETRY_MS else remaining.coerceAtLeast(1_000L))
    }

    override fun onCreateOptionsMenu(menu: Menu): Boolean {
        menuInflater.inflate(R.menu.main_menu, menu)
        return true
    }

    override fun onPrepareOptionsMenu(menu: Menu): Boolean {
        val state = viewModel.state.value
        listOf(R.id.action_language, R.id.action_preferences, R.id.action_about).forEach {
            menu.findItem(it)?.isEnabled = menuEnabled
        }
        menu.findItem(R.id.action_diagnostics)?.isVisible = state.diagnosticsAvailable
        return super.onPrepareOptionsMenu(menu)
    }

    override fun onOptionsItemSelected(item: MenuItem): Boolean = when (item.itemId) {
        R.id.action_help -> { showHelp(); true }
        R.id.action_language -> { showLanguageSelector(); true }
        R.id.action_preferences -> { showPreferences(); true }
        R.id.action_diagnostics -> { showDiagnostics(); true }
        R.id.action_about -> { showAbout(); true }
        else -> super.onOptionsItemSelected(item)
    }

    /**
     * Aviso que bloquea una acción. Si ofrece una acción no desaparece solo:
     * se queda hasta que la persona la usa o lo descarta (WCAG 2.2.1).
     */
    private fun showMessage(message: Int, action: Int? = null, onAction: (() -> Unit)? = null) {
        if (action != null && onAction != null) {
            Snackbar.make(binding.rootLayout, message, Snackbar.LENGTH_INDEFINITE)
                .setAction(action) { onAction() }.show()
        } else {
            Snackbar.make(binding.rootLayout, message, Snackbar.LENGTH_LONG).show()
        }
    }

    override fun onStop() {
        // Con un selector del sistema abierto la persona sigue trabajando: el plazo
        // empieza a contar tras un margen, por si deja el selector abierto y se va.
        backgroundSince = SystemClock.elapsedRealtime() + if (systemPickerOpen) PICKER_GRACE_MS else 0L
        scheduleCloseCheck()
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
        listOf(toggleSigningOptionsButton to signingOptionsGroup, toggleOtherToolsButton to otherToolsGroup)
            .forEach { (toggle, group) ->
                toggle.setOnClickListener {
                    if (!expandedTools.remove(group.id)) expandedTools += group.id
                    renderMainToggles()
                }
            }
        renderMainToggles()
        toggleVerificationTechnicalButton.setOnClickListener {
            verificationTechnicalExpanded = !verificationTechnicalExpanded
            renderVerification(viewModel.state.value)
        }
        exportReportButton.setOnClickListener { viewModel.exportVerificationReport() }
        exportReportHtmlButton.setOnClickListener { viewModel.exportVerificationReport(printable = true) }
        useCosignButton.setOnClickListener { viewModel.acceptCoSignSuggestion() }
        signatureAction.onItemSelected = { updateSigningSettings() }
        signatureProfile.onItemSelected = { updateSigningSettings() }
        signatureFormat.onItemSelected = { renderSigningSummary(viewModel.state.value) }
        tsaEnabled.setOnCheckedChangeListener { _, _ -> updateSigningSettings() }
        tsaUrl.doAfterTextChanged { updateSigningSettings() }
        selectCertificateFileButton.setOnClickListener {
            stopDnieReading()
            // Con varias identidades el DNIe sigue abierto junto al nuevo PKCS#12.
            if (dnieSession != null && !viewModel.state.value.canKeepSeveralIdentities) {
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
            // Contraseña vacía o incorrecta: el modelo la marca en el propio campo.
            viewModel.importCertificate(password)
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
            if (usesDnie()) {
                val document = viewModel.state.value.document
                if (sealSettings.enabled && document != null && isPdf(document) && sealPageInfo == null) {
                    showSealEditor(continueSigning = true)
                } else {
                    showDniePinDialog()
                }
            } else signWithSealIfSelected()
        }
        verifyButton.setOnClickListener { viewModel.verify() }
        retrySaveButton.setOnClickListener { viewModel.retryPendingOutput() }
        discardPendingOutputButton.setOnClickListener { confirmDiscard() }
        configureTools()
        configureFormats()
        configureDocuments()
        configureCertificatePanel()
    }

    private fun selectDefaultFormat() {
        val index = formatMenu.indexOf(viewModel.state.value.settings.defaultFormat)
        if (index >= 0) binding.signatureFormat.select(index)
    }

    /** Se firma con el DNIe solo si es el certificado elegido y su lectura NFC sigue abierta. */
    private fun usesDnie(): Boolean = dnieSession != null && viewModel.state.value.certificateExternal

    private fun closeIdentity(identityId: String) {
        val state = viewModel.state.value
        if (!state.canChangeIdentity) return
        val identity = state.identities.firstOrNull { it.id == identityId } ?: return
        if (identity.external) {
            stopDnieReading()
            dnieSession?.close()
            dnieSession = null
        }
        viewModel.closeIdentity(identityId)
    }

    private fun configureCertificatePanel() = with(binding.certificatePanel) {
        certificateKindFilter.setItems(
            listOf(getString(R.string.cert_filter_all)) + CertificateFilter.KINDS.map { getString(CertificateText.kindLabel(it)) })
        certificateKindFilter.onItemSelected = { updateCertificateFilter() }
        certificateFilter.doAfterTextChanged { updateCertificateFilter() }
        checkCertificateOnlineButton.setOnClickListener { viewModel.checkCertificateOnline() }
    }

    private fun updateCertificateFilter() {
        if (updatingCertificateFilter) return
        val panel = binding.certificatePanel
        val position = panel.certificateKindFilter.selectedItemPosition
        viewModel.updateCertificateFilter(panel.certificateFilter.text?.toString().orEmpty(),
            if (position <= 0) "" else CertificateFilter.KINDS.getOrElse(position - 1) { "" })
    }

    private fun renderCertificatePanel(state: MainUiState) = with(binding.certificatePanel) {
        val available = state.certificatePanelAvailable && state.certificate != null
        certificateFilterGroup.visibility = if (available && state.showsCertificateFilter) View.VISIBLE else View.GONE
        // Con la lista de certificados abiertos, el detalle del elegido va junto a su fila.
        val shown = when {
            state.showsIdentityList -> emptyList()
            state.showsCertificateFilter -> state.filteredCertificates
            else -> state.certificateDetails
        }
        if (state.showsCertificateFilter) {
            val matching = state.filteredCertificates.size
            certificateFilterCount.text = if (matching == 0) getString(R.string.cert_filter_none) else
                resources.getQuantityString(R.plurals.cert_filter_count, matching, matching)
        }
        certificateDetail.visibility = if (available && shown.isNotEmpty()) View.VISIBLE else View.GONE
        // Solo la línea de caducidad va en color de aviso; el resto del detalle, en el normal.
        certificateDetail.text = SpannableStringBuilder().apply {
            shown.forEachIndexed { index, detail ->
                if (index > 0) append("\n\n")
                append(CertificateText.styled(this@MainActivity, detail))
            }
        }
        IdentityPanel.render(this@MainActivity, state, identityGroup, identityListTitle, identityList,
            onSelect = viewModel::selectIdentity, onClose = ::closeIdentity)
        identityAddHint.visibility = if (state.canKeepSeveralIdentities && state.identities.size == 1 &&
            state.canReplaceSelection) View.VISIBLE else View.GONE
        checkCertificateOnlineButton.visibility = if (state.certificate != null &&
            PlatformServicesVisibility.online(state)) View.VISIBLE else View.GONE
        checkCertificateOnlineButton.isEnabled = state.canCheckCertificateOnline
        updatingCertificateFilter = true
        if (certificateFilter.text?.toString() != state.certificateFilter) certificateFilter.setText(state.certificateFilter)
        certificateKindFilter.select(CertificateFilter.KINDS.indexOf(state.certificateKindFilter) + 1)
        updatingCertificateFilter = false
    }

    private fun renderMainToggles() = with(binding) {
        listOf(toggleSigningOptionsButton to signingOptionsGroup, toggleOtherToolsButton to otherToolsGroup)
            .forEach { (toggle, group) ->
                val expanded = group.id in expandedTools
                group.visibility = if (expanded) View.VISIBLE else View.GONE
                toggle.setIconResource(if (expanded) R.drawable.ic_expand_less else R.drawable.ic_expand_more)
                ViewCompat.setStateDescription(toggle,
                    getString(if (expanded) R.string.state_expanded else R.string.state_collapsed))
            }
        // El resumen solo hace falta con las opciones plegadas: abiertas, ya se ven los campos.
        signingOptionsSummary.visibility =
            if (signingOptionsGroup.id in expandedTools) View.GONE else View.VISIBLE
    }

    /** Resumen de las opciones avanzadas cuando están plegadas. */
    private fun renderSigningSummary(state: MainUiState) {
        val action = when (state.signatureAction) {
            "cosign" -> R.string.action_cosign
            "countersign" -> R.string.action_countersign
            else -> R.string.action_sign
        }
        binding.signingOptionsSummary.text = getString(R.string.signing_options_summary, getString(action),
            getString(FormatPolicy.label(selectedSignatureFormat())),
            getString(if (state.tsaEnabled) R.string.summary_tsa else R.string.summary_no_tsa))
    }

    /** Descartar es irreversible: se confirma antes, con el botón seguro por defecto. */
    private fun confirmDiscard() {
        val signature = viewModel.state.value.pendingKind == PendingKind.SIGNATURE
        MaterialAlertDialogBuilder(this)
            .setTitle(if (signature) R.string.discard_confirm_title else R.string.discard_tool_confirm_title)
            .setMessage(if (signature) R.string.discard_confirm_message else R.string.discard_tool_confirm_message)
            .setPositiveButton(if (signature) R.string.discard_keep_signature else R.string.discard_keep_result, null)
            .setNegativeButton(R.string.discard_confirm_action) { _, _ -> viewModel.discardPendingOutput() }
            .show()
    }

    /** El desplegable ofrece solo los formatos que declara el núcleo. */
    private fun configureFormats() {
        val declared = viewModel.state.value.signingFormats
        formatMenu = listOf("auto") + FormatPolicy.MENU_ORDER.filter { it in declared }
        binding.signatureFormat.setItems(formatMenu.map { getString(FormatPolicy.label(it)) })
    }

    private fun configureDocuments() = with(binding.documents) {
        listOf(toggleVerifactuButton to verifactuGroup, toggleEniButton to eniGroup).forEach { (toggle, group) ->
            toggle.setOnClickListener {
                if (!expandedTools.remove(group.id)) expandedTools += group.id
                renderDocumentToggles()
            }
        }
        renderDocumentToggles()
        selectVerifactuButton.setOnClickListener {
            try {
                openVeriFactuRecords.launch(arrayOf("text/xml", "application/xml", "application/octet-stream"))
            } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
        clearVerifactuButton.setOnClickListener { viewModel.clearVeriFactuRecords() }
        checkVerifactuButton.setOnClickListener { viewModel.checkVeriFactu() }
        exportVerifactuButton.setOnClickListener { viewModel.exportVeriFactuReport() }
        readQrButton.setOnClickListener { viewModel.readVeriFactuQr(qrUrl.text?.toString().orEmpty()) }
        takeQrPhotoButton.setOnClickListener {
            try {
                takeQrPhoto.launch(QrCapture.prepare(this@MainActivity))
            } catch (_: ActivityNotFoundException) {
                QrCapture.clear(this@MainActivity)
                showMessage(R.string.qr_camera_unavailable)
            } catch (_: RuntimeException) {
                QrCapture.clear(this@MainActivity)
                showMessage(R.string.qr_camera_unavailable)
            }
        }
        chooseQrImageButton.setOnClickListener {
            try {
                openQrImage.launch(arrayOf("image/*"))
            } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
        queryAeatButton.setOnClickListener { viewModel.queryAeat() }
        qrUrl.doAfterTextChanged {
            val qr = viewModel.state.value.veriFactuQr
            if (qr != null && it?.toString()?.trim() != qr.url) viewModel.clearVeriFactuQr()
        }
        val catalogs = viewModel.eniCatalogs()
        eniStates = catalogs.documentStates
        eniTypes = catalogs.documentTypes
        eniOrigin.setItems(listOf(getString(R.string.eni_origin_administration), getString(R.string.eni_origin_citizen)))
        eniState.setItems(eniStates.map(::eniCodeLabel))
        eniDocumentType.setItems(eniTypes.map(::eniCodeLabel))
        if (eniDocumentType.text.isNullOrEmpty() || eniDocumentType.selectedItemPosition == 0) {
            eniDocumentType.select(eniTypes.indexOf("TD99").coerceAtLeast(0))
        }
        eniCaptureDateButton.setOnClickListener { showEniDatePicker() }
        eniCaptureDateClearButton.setOnClickListener { viewModel.updateEniCaptureDate(null) }
        createEniButton.setOnClickListener { createEni() }
        validateEniButton.setOnClickListener {
            try {
                openEniDocument.launch(arrayOf("text/xml", "application/xml", "application/octet-stream"))
            } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
    }

    /** Código oficial seguido de su descripción del catálogo del motor. */
    private fun eniCodeLabel(code: String): String {
        val key = "eni.codigo.$code"
        val description = EngineText.resolve(this, key)
        return if (description == key) code else getString(R.string.eni_code_label, code, description)
    }

    private fun showEniDatePicker() {
        if (supportFragmentManager.findFragmentByTag(ENI_DATE_PICKER) != null) return
        val picker = MaterialDatePicker.Builder.datePicker()
            .setTitleText(R.string.eni_capture_date_title)
            .setSelection(viewModel.state.value.eniCaptureDate ?: MaterialDatePicker.todayInUtcMilliseconds())
            .setCalendarConstraints(CalendarConstraints.Builder().setValidator(DateValidatorPointBackward.now()).build())
            .build()
        picker.addOnPositiveButtonClickListener { viewModel.updateEniCaptureDate(it) }
        picker.show(supportFragmentManager, ENI_DATE_PICKER)
    }

    private fun createEni() = with(binding.documents) {
        val organs = EniForm.organs(eniOrgans.text?.toString().orEmpty())
        if (!EniForm.organsValid(organs)) {
            eniOrgansLayout.error = EngineText.resolve(this@MainActivity, "eni.validacion.dir3")
            return@with
        }
        eniOrgansLayout.error = null
        viewModel.createEni(EniRequest(
            organs = organs,
            origin = if (eniOrigin.selectedItemPosition == 1) "ciudadano" else "administracion",
            state = eniStates.getOrElse(eniState.selectedItemPosition) { "EE01" },
            documentType = eniTypes.getOrElse(eniDocumentType.selectedItemPosition) { "TD99" },
            identifier = eniIdentifier.text?.toString()?.trim().orEmpty(),
            sourceIdentifier = eniSource.text?.toString()?.trim().orEmpty(),
            captureDate = EniForm.captureDate(viewModel.state.value.eniCaptureDate),
            contentFormat = eniContentFormat.text?.toString()?.trim().orEmpty(),
        ))
    }

    private fun renderDocumentToggles() = with(binding.documents) {
        listOf(toggleVerifactuButton to verifactuGroup, toggleEniButton to eniGroup).forEach { (toggle, group) ->
            val expanded = group.id in expandedTools
            group.visibility = if (expanded) View.VISIBLE else View.GONE
            toggle.setIconResource(if (expanded) R.drawable.ic_expand_less else R.drawable.ic_expand_more)
            ViewCompat.setStateDescription(toggle,
                getString(if (expanded) R.string.state_expanded else R.string.state_collapsed))
        }
    }

    private fun renderDocuments(state: MainUiState) = with(binding.documents) {
        val any = state.verifactuAvailable || state.eniDocumentAvailable || state.eniValidateAvailable
        documentsHelper.setText(if (any) R.string.documents_helper else R.string.documents_unavailable)
        val idle = state.canReplaceSelection
        toggleVerifactuButton.visibility = if (state.verifactuAvailable) View.VISIBLE else View.GONE
        if (!state.verifactuAvailable) verifactuGroup.visibility = View.GONE
        toggleEniButton.visibility = if (state.eniDocumentAvailable || state.eniValidateAvailable) View.VISIBLE else View.GONE
        if (!state.eniDocumentAvailable && !state.eniValidateAvailable) eniGroup.visibility = View.GONE
        selectVerifactuButton.isEnabled = state.verifactuAvailable && idle
        verifactuSummary.text = if (state.verifactuRecords.isEmpty()) getString(R.string.verifactu_none) else
            resources.getQuantityString(R.plurals.verifactu_selected, state.verifactuRecords.size, state.verifactuRecords.size) +
                "\n" + state.verifactuRecords.joinToString("\n") { it.summaryText() }
        clearVerifactuButton.visibility = if (state.verifactuRecords.isEmpty()) View.GONE else View.VISIBLE
        clearVerifactuButton.isEnabled = idle
        checkVerifactuButton.isEnabled = state.canCheckVeriFactu
        exportVerifactuButton.visibility = if (state.veriFactuReport != null) View.VISIBLE else View.GONE
        exportVerifactuButton.isEnabled = state.canExportVeriFactu
        qrGroup.visibility = if (state.qrReadAvailable) View.VISIBLE else View.GONE
        qrUrlLayout.isEnabled = idle
        readQrButton.isEnabled = state.canReadQr
        val hasCamera = packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)
        takeQrPhotoButton.visibility = if (state.qrImageAvailable && hasCamera) View.VISIBLE else View.GONE
        takeQrPhotoButton.isEnabled = state.canReadQrImage
        chooseQrImageButton.visibility = if (state.qrImageAvailable) View.VISIBLE else View.GONE
        chooseQrImageButton.isEnabled = state.canReadQrImage
        // El QR leído de una imagen muestra su URL en el campo, como si se hubiera pegado.
        state.veriFactuQr?.let { qr -> if (qrUrl.text?.toString()?.trim() != qr.url) qrUrl.setText(qr.url) }
        queryAeatButton.visibility = if (PlatformServicesVisibility.aeat(state)) View.VISIBLE else View.GONE
        queryAeatButton.isEnabled = state.canQueryAeat
        val qrText = buildList {
            state.veriFactuQr?.let { add(QrText.lines(it).resolve(this@MainActivity)) }
            state.qrError?.let { add(it.resolve(this@MainActivity)) }
            if (state.aeatResponse.isNotEmpty()) add(getString(R.string.qr_aeat_response) + "\n" + state.aeatResponse)
        }.joinToString("\n\n")
        qrResult.text = qrText
        qrResult.visibility = if (qrText.isEmpty()) View.GONE else View.VISIBLE
        qrResult.setTextColor(ContextCompat.getColor(this@MainActivity,
            if (state.qrError != null) R.color.error else R.color.on_surface))
        listOf(eniOrgansLayout, eniSourceLayout, eniIdentifierLayout, eniContentFormatLayout).forEach { it.isEnabled = idle }
        listOf(eniOrigin, eniState, eniDocumentType).forEach { it.isEnabled = idle }
        eniCaptureDateSummary.text = state.eniCaptureDate?.let {
            val format = DateFormat.getDateInstance(DateFormat.LONG).apply { timeZone = TimeZone.getTimeZone("UTC") }
            getString(R.string.eni_capture_date_value, format.format(Date(it)))
        } ?: getString(R.string.eni_capture_date_now)
        eniCaptureDateButton.isEnabled = idle
        eniCaptureDateClearButton.visibility = if (state.eniCaptureDate == null) View.GONE else View.VISIBLE
        eniCaptureDateClearButton.isEnabled = idle
        createEniButton.visibility = if (state.eniDocumentAvailable) View.VISIBLE else View.GONE
        createEniButton.isEnabled = state.canCreateEni
        validateEniButton.visibility = if (state.eniValidateAvailable) View.VISIBLE else View.GONE
        validateEniButton.isEnabled = state.canValidateEni
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
        hashAlgorithm.onItemSelected = { updateToolSettings() }
        hashFormat.onItemSelected = { updateToolSettings() }
        protectionContainer.onItemSelected = { updateToolSettings() }
        protectForMe.setOnCheckedChangeListener { _, _ -> updateToolSettings() }
        selectBatchButton.setOnClickListener {
            try { openBatchDocuments.launch(arrayOf("*/*")) } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
        clearBatchButton.setOnClickListener { viewModel.clearBatchDocuments() }
        signBatchButton.setOnClickListener { wave4.signBatch(selectedSignatureFormat(), batchSealPlanner()) }
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
        protectSignButton.setOnClickListener { dnieAccess.withPin(hold = false) { protectWithKey(sign = true) } }
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
        batchHelperText.setText(if (state.externalBatchAvailable || state.batchSealAvailable)
            R.string.batch_helper_wave4 else R.string.batch_helper)
        hashAlgorithm.isEnabled = idle
        hashFormat.isEnabled = idle
        createHashButton.isEnabled = state.canHash
        checkHashButton.isEnabled = state.canHash
        protectionContainer.isEnabled = idle
        val transient = state.usesTransientKey
        protectForMe.visibility = if (transient) View.GONE else View.VISIBLE
        protectForMe.isEnabled = idle && state.certificate != null && !state.certificateExternal
        addRecipientButton.visibility = protectForMe.visibility
        addRecipientButton.isEnabled = state.canUseTools && state.recipients.size < ToolsPolicy.MAX_RECIPIENTS
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
        hashAlgorithm.select(ToolsPolicy.HASH_ALGORITHMS.indexOf(state.hashAlgorithm).coerceAtLeast(0))
        hashFormat.select(ToolsPolicy.HASH_FORMATS.indexOf(state.hashFormat).coerceAtLeast(0))
        protectionContainer.select(ToolsPolicy.CONTAINERS.indexOf(state.protectionContainer).coerceAtLeast(0))
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
                            UiEffect.SaveVerificationHtml -> try {
                                createVerificationHtml.launch(getString(R.string.verification_report_html_filename))
                            } catch (_: RuntimeException) { viewModel.cancelReportExport() }
                            UiEffect.SaveVeriFactuReport -> try {
                                createVeriFactuReport.launch(getString(R.string.verifactu_report_filename))
                            } catch (_: RuntimeException) { viewModel.cancelReportExport() }
                            is UiEffect.OpenRelease -> openRelease(effect.url)
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
            // Las posiciones por página y el CSV pertenecen al documento anterior.
            sealSettings = sealSettings.copy(placements = emptyMap(), csvCode = "")
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
        // Con certificados ya abiertos, «No ha elegido ningún fichero» solo confunde.
        certificateFileSummary.visibility =
            if (state.certificateFile == null && state.identities.isNotEmpty()) View.GONE else View.VISIBLE
        certificateSummary.text = state.certificate?.let { certificate ->
            getString(
                R.string.certificate_summary,
                certificate.subject,
                certificate.issuer.ifBlank { getString(R.string.unknown_value) },
            )
        } ?: getString(R.string.no_certificate)
        // Con varios abiertos, la lista ya dice cuál está elegido.
        certificateSummary.visibility = if (state.showsIdentityList) View.GONE else View.VISIBLE

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
        val passwordError = state.certificatePasswordError?.resolve(this@MainActivity)
        if (certificatePasswordLayout.error?.toString() != passwordError) {
            certificatePasswordLayout.error = passwordError
            if (passwordError != null) certificatePassword.requestFocus()
        }
        importCertificateButton.visibility = certificateImportVisibility
        forgetCertificateButton.isEnabled = state.canForgetCertificate
        forgetCertificateButton.visibility =
            if (state.certificate == null) View.GONE else View.VISIBLE
        forgetCertificateButton.setText(if (state.identities.size > 1) R.string.forget_all_certificates else R.string.forget_certificate)
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
        verifyButton.isEnabled = state.canVerify
        certificatePasswordLayout.isEnabled = !state.busy
        signatureFormat.isEnabled = state.canReplaceSelection
        signatureAction.isEnabled = state.canReplaceSelection
        signatureProfile.isEnabled = state.canReplaceSelection
        tsaEnabled.isEnabled = state.canReplaceSelection
        tsaUrl.isEnabled = state.canReplaceSelection && state.tsaEnabled
        tsaUrlLayout.visibility = if (state.tsaEnabled) View.VISIBLE else View.GONE
        if (menuEnabled != state.canReplaceSelection) {
            menuEnabled = state.canReplaceSelection
            invalidateOptionsMenu()
        }
        updatingSigningControls = true
        signatureAction.select(listOf("sign", "cosign", "countersign").indexOf(state.signatureAction).coerceAtLeast(0))
        signatureProfile.select(listOf("baseline", "t", "lt", "lta").indexOf(state.signatureProfile).coerceAtLeast(0))
        tsaEnabled.isChecked = state.tsaEnabled
        if (tsaUrl.text.toString() != state.tsaUrl) tsaUrl.setText(state.tsaUrl)
        updatingSigningControls = false
        useCosignButton.visibility = if (state.coSignSuggested) View.VISIBLE else View.GONE
        useCosignButton.isEnabled = state.canReplaceSelection
        renderVerification(state)
        exportReportButton.visibility = if (state.verification?.reportJson?.isNotEmpty() == true) View.VISIBLE else View.GONE
        exportReportButton.isEnabled = state.canExportReport
        exportReportHtmlButton.visibility = if (state.verification?.reportHtml?.isNotEmpty() == true) View.VISIBLE else View.GONE
        exportReportHtmlButton.isEnabled = state.canExportReport
        progressContainer.visibility = if (state.busy) View.VISIBLE else View.GONE
        pendingSaveActions.visibility = if (state.canRetryPendingOutput) View.VISIBLE else View.GONE
        retrySaveButton.isEnabled = state.canRetryPendingOutput
        discardPendingOutputButton.isEnabled = state.canDiscardPendingOutput
        renderTools(state)
        renderDocuments(state)
        renderCertificatePanel(state)
        renderDiagnostics(state)
        renderUpdateCheck(state)
        renderSigningSummary(state)
        renderHints(state)
        wave4.render(state)

        when (val result = state.result) {
            OperationResult.Idle -> {
                resultTitle.setText(R.string.result_idle)
                resultTitle.setTextColor(
                    ContextCompat.getColor(this@MainActivity, R.color.on_surface),
                )
                resultDetail.visibility = View.GONE
            }
            is OperationResult.Success -> {
                // Tras verificar, el título es el veredicto: no hay otra conclusión que lo contradiga.
                val verdict = (result.detail as? UiText.Verification)?.let(VerificationCard::verdict)
                resultTitle.text = if (verdict != null) getString(VerificationCard.title(verdict))
                    else result.title.resolve(this@MainActivity)
                val color = verdict?.let(VerificationCard::color) ?: R.color.primary
                resultTitle.setTextColor(ContextCompat.getColor(this@MainActivity, color))
                renderDetail(if (result.detail is UiText.Verification) null else result.detail?.resolve(this@MainActivity))
            }
            is OperationResult.Notice -> {
                resultTitle.text = result.detail.resolve(this@MainActivity)
                resultTitle.setTextColor(ContextCompat.getColor(this@MainActivity, R.color.on_surface))
                renderDetail(null)
            }
            is OperationResult.Error -> {
                // Durante una operación el PIN sigue en uso (lote): no se toca.
                if (!state.busy) {
                    dnieSession?.consumeSigningError()?.let { error ->
                        viewModel.reportDnieError(error, dnieSession?.retriesLeft() ?: -1)
                        return@with
                    }
                    dnieSession?.clearPin()
                }
                resultTitle.setText(R.string.result_error)
                resultTitle.setTextColor(ContextCompat.getColor(this@MainActivity, R.color.error))
                renderDetail(result.detail.resolve(this@MainActivity))
            }
        }
        revealResult(state.result)
    }

    /**
     * Tras cada operación el resultado se desplaza a la vista; TalkBack lo
     * anuncia por su región viva sin forzar el foco.
     */
    private fun revealResult(result: OperationResult) {
        val previous = lastResult
        lastResult = result
        if (previous == null || result === previous || result == OperationResult.Idle) return
        binding.contentScroll.post {
            val top = binding.contentColumn.top + binding.resultCard.top
            binding.contentScroll.smoothScrollTo(0, (top - resources.getDimensionPixelSize(R.dimen.control_spacing)).coerceAtLeast(0))
        }
    }

    /** Explica qué falta cuando Firmar, Verificar o el DNIe están desactivados. */
    private fun renderHints(state: MainUiState) = with(binding) {
        backendStatusCard.visibility = if (state.backend.available) View.GONE else View.VISIBLE
        val action = when {
            state.busy || state.awaitingSave -> null
            !state.backend.available -> R.string.hint_unavailable
            state.document == null -> R.string.hint_need_document
            state.certificate == null -> R.string.hint_need_certificate
            else -> null
        }
        actionHint.visibility = if (action == null) View.GONE else View.VISIBLE
        if (action != null) actionHint.setText(action)
        // Si el botón del DNIe está desactivado, siempre se explica por qué.
        val dnie = when {
            state.busy || state.awaitingSave -> null
            nfcAdapter == null -> R.string.dnie_hint_no_nfc
            !state.backend.available -> R.string.hint_unavailable
            state.document == null -> R.string.dnie_hint_document_first
            else -> null
        }
        dnieHint.visibility = if (dnie == null) View.GONE else View.VISIBLE
        if (dnie != null) dnieHint.setText(dnie)
        selectDnieNfcButton.isEnabled = state.canReplaceSelection && state.backend.available &&
            nfcAdapter != null && state.document != null
        val certificate = state.certificate
        openCertificateStatus.visibility = if (certificate != null && !state.certificateExternal) View.VISIBLE else View.GONE
        if (certificate != null && !state.certificateExternal) {
            val minutes = state.settings.sessionTimeoutMinutes
            openCertificateStatus.text = if (minutes > 0) getString(R.string.certificate_open_status, certificate.subject,
                resources.getQuantityString(R.plurals.minutes, minutes, minutes))
            else getString(R.string.certificate_open_status_manual, certificate.subject)
        }
    }

    /**
     * Verificación de la última operación: resumen en lenguaje llano y, plegadas,
     * las evidencias técnicas. Tras firmar, la primera línea da el veredicto de
     * la firma creada; tras verificar, el veredicto ya es el título.
     */
    private fun renderVerification(state: MainUiState) = with(binding) {
        val verification = state.verification?.toUiText()
        if (state.verification !== lastVerification) {
            lastVerification = state.verification
            verificationTechnicalExpanded = false
        }
        val shown = verification != null || state.postSignVerificationFailed
        verificationDetail.visibility = if (shown) View.VISIBLE else View.GONE
        val text = SpannableStringBuilder()
        if (verification != null) {
            val isResult = (state.result as? OperationResult.Success)?.detail is UiText.Verification
            if (!isResult) {
                val verdict = VerificationCard.verdict(verification)
                val start = text.length
                text.append(getString(R.string.verification_post_sign, getString(VerificationCard.title(verdict))))
                text.setSpan(ForegroundColorSpan(ContextCompat.getColor(this@MainActivity, VerificationCard.color(verdict))),
                    start, text.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                text.setSpan(StyleSpan(Typeface.BOLD), start, text.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                text.append("\n")
            }
            text.append(getString(R.string.verification_document, state.verifiedDocumentName)).append("\n")
            text.append(verification.resolve(this@MainActivity))
        } else if (state.postSignVerificationFailed) {
            text.append(getString(R.string.verification_document, state.verifiedDocumentName)).append("\n")
            val start = text.length
            text.append(getString(R.string.post_sign_verification_failed))
            text.setSpan(ForegroundColorSpan(ContextCompat.getColor(this@MainActivity, R.color.status_warning)),
                start, text.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }
        verificationDetail.text = text
        toggleVerificationTechnicalButton.visibility = if (verification != null) View.VISIBLE else View.GONE
        val expanded = verification != null && verificationTechnicalExpanded
        toggleVerificationTechnicalButton.setIconResource(if (expanded) R.drawable.ic_expand_less else R.drawable.ic_expand_more)
        ViewCompat.setStateDescription(toggleVerificationTechnicalButton,
            getString(if (expanded) R.string.state_expanded else R.string.state_collapsed))
        verificationTechnical.visibility = if (expanded) View.VISIBLE else View.GONE
        verificationTechnical.text = if (expanded) VerificationCard.technical(this@MainActivity, verification!!) else ""
    }

    private fun renderDetail(detail: String?) = with(binding.resultDetail) {
        text = detail.orEmpty()
        visibility = if (detail.isNullOrBlank()) View.GONE else View.VISIBLE
    }

    private fun SelectedFile.summaryText(): String = getString(
        R.string.document_summary,
        displayName,
        typeLabel(),
        formatSize(sizeBytes),
    )

    /** Tipo corto para la persona (PDF, XML…), no el tipo MIME. */
    private fun SelectedFile.typeLabel(): String {
        val extension = displayName.substringAfterLast('.', "").takeIf { it.length in 1..5 && it.all(Char::isLetterOrDigit) }
        return extension?.uppercase(java.util.Locale.ROOT)
            ?: android.webkit.MimeTypeMap.getSingleton().getExtensionFromMimeType(mimeType)?.uppercase(java.util.Locale.ROOT)
            ?: getString(R.string.unknown_value)
    }

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
            .setSingleChoiceItems(R.array.language_names, LanguageTags.indexFor(current, tags.toList())) { dialog, index ->
                dialog.dismiss()
                AppCompatDelegate.setApplicationLocales(LocaleListCompat.forLanguageTags(tags[index]))
            }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    private fun showAbout() {
        val engine = viewModel.engineVersion.takeIf { it.isNotBlank() && it != "development" }
            ?: getString(R.string.unknown_value)
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.about_title)
            .setMessage(getString(R.string.about_content, BuildConfig.VERSION_NAME, engine, AppLinks.CONTACT_EMAIL) + "\n\n" +
                getString(R.string.about_release_link, AppLinks.RELEASES))
            .setPositiveButton(R.string.help_close, null)
            .setNeutralButton(R.string.about_release_notes) { _, _ ->
                MaterialAlertDialogBuilder(this).setTitle(R.string.about_release_notes)
                    .setMessage(getString(R.string.release_notes_wave3) + "\n\n" + getString(R.string.release_notes_wave4) + "\n\n" + getString(R.string.release_notes_wave2b) +
                        "\n\n" + getString(R.string.release_notes_content))
                    .setPositiveButton(R.string.help_close, null).show()
            }
            .apply {
                if (viewModel.state.value.updateCheckAvailable) {
                    setNegativeButton(R.string.about_check_updates) { _, _ -> viewModel.checkUpdate(BuildConfig.VERSION_NAME) }
                }
            }
            .show()
        dialog.findViewById<android.widget.TextView>(android.R.id.message)?.let {
            Linkify.addLinks(it, Linkify.EMAIL_ADDRESSES or Linkify.WEB_URLS)
            it.movementMethod = LinkMovementMethod.getInstance()
        }
    }

    private fun selectedSignatureFormat(): String =
        formatMenu.getOrElse(binding.signatureFormat.selectedItemPosition) { "auto" }

    private fun isPdf(file: SelectedFile): Boolean =
        file.mimeType.equals("application/pdf", ignoreCase = true) || file.displayName.endsWith(".pdf", ignoreCase = true)

    /**
     * [continueSigning]: el editor se abrió al pulsar «Firmar documento»; tras
     * «Aplicar sello» la firma sigue sola. Si no, se dice cómo terminar.
     */
    private fun showSealEditor(continueSigning: Boolean = false) {
        if (sealEditorOpen) return
        val document = viewModel.state.value.document
        if (document == null || !isPdf(document)) return
        if (viewModel.state.value.certificate == null) {
            showMessage(R.string.seal_certificate_required)
            return
        }
        sealEditorOpen = true
        sealEditor = SealEditorDialog(this, viewModel, document, sealSettings, sealImageFile,
            chooseImage = {
                try { openSealImage.launch(arrayOf("image/png", "image/jpeg")) }
                catch (_: RuntimeException) {
                    showMessage(R.string.error_picker_unavailable)
                }
            },
            onSaved = { settings, count, width, height ->
                sealSettings = settings
                sealPageInfo = Triple(count, width, height)
                sealPreferences.save(settings)
                // Tras cerrarse el editor: la firma sigue o se explica el paso siguiente.
                binding.rootLayout.post {
                    when {
                        !continueSigning -> showMessage(R.string.seal_placed_press_sign)
                        !viewModel.state.value.canSign -> Unit
                        usesDnie() -> showDniePinDialog()
                        else -> signWithSealIfSelected()
                    }
                }
            },
            onClosed = {
                sealEditorOpen = false
                sealEditor = null
            },
            onError = { showMessage(it) })
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
            showMessage(R.string.seal_pdf_only)
            return
        }
        val pageInfo = sealPageInfo
        if (pageInfo == null) {
            dnieSession?.clearPin()
            showSealEditor(continueSigning = true)
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
            showMessage(R.string.seal_invalid_settings)
        }
    }

    private fun importSealImage(uri: Uri) {
        try {
            val bytes = contentResolver.openInputStream(uri)?.use {
                DocumentPolicy.readBounded(it, 2 * 1024 * 1024)
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
            showMessage(R.string.seal_image_error)
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
            showMessage(R.string.dnie_document_first, R.string.snack_choose_document) {
                try { openDocument.launch(arrayOf("*/*")) } catch (_: RuntimeException) { viewModel.reportPickerError() }
            }
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
        val layout = secretInputLayout(R.string.dnie_can_label, field, password = false)
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.dnie_can_title)
            .setMessage(R.string.dnie_can_help)
            .setView(layout.parent as View)
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

    private fun showDniePinDialog(hold: Boolean = false, onPin: () -> Unit = ::signWithSealIfSelected) {
        val session = dnieSession ?: return
        val field = TextInputEditText(this).apply {
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
            filters = arrayOf(android.text.InputFilter.LengthFilter(16))
            isSaveEnabled = false
            importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_NO_EXCLUDE_DESCENDANTS
        }
        val layout = secretInputLayout(R.string.dnie_pin_label, field, password = true)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.dnie_pin_title)
            .setMessage(R.string.dnie_pin_help)
            .setView(layout.parent as View)
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
                    if (hold) session.beginOperation(pin) else session.setPin(pin)
                    dialog.dismiss()
                    onPin()
                } finally {
                    pin.fill('\u0000')
                }
            }
        }
        dialog.show()
    }

    private fun showPreferences() {
        if (!viewModel.state.value.canReplaceSelection) return
        val dialogBinding = DialogPreferencesBinding.inflate(layoutInflater)
        val current = viewModel.state.value.settings
        val formats = formatMenu
        with(dialogBinding) {
            prefFormat.setItems(formats.map { getString(FormatPolicy.label(it)) })
            prefFormat.select(formats.indexOf(current.defaultFormat).coerceAtLeast(0))
            prefProfile.select(AppSettings.PROFILES.indexOf(current.defaultProfile).coerceAtLeast(0))
            prefSessionTimeout.setItems(AppSettings.TIMEOUTS.map {
                if (it == 0) getString(R.string.timeout_never) else resources.getQuantityString(R.plurals.minutes, it, it)
            })
            prefSessionTimeout.select(AppSettings.TIMEOUTS.indexOf(current.sessionTimeoutMinutes).coerceAtLeast(0))
            prefTsaEnabled.isChecked = current.tsaEnabled
            prefTsaUrl.setText(current.tsaUrl)
            prefTsaUrlLayout.isEnabled = current.tsaEnabled
            prefTsaEnabled.setOnCheckedChangeListener { _, checked -> prefTsaUrlLayout.isEnabled = checked }
            prefOutputName.select(OutputNames.POLICIES.indexOf(current.outputName).coerceAtLeast(0))
            prefTheme.select(AppSettings.THEMES.indexOf(current.theme).coerceAtLeast(0))
        }
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.preferences_title)
            .setView(dialogBinding.root)
            .setPositiveButton(R.string.preferences_save, null)
            .setNegativeButton(R.string.preferences_cancel, null)
            .create()
        dialogBinding.prefRestore.setOnClickListener {
            MaterialAlertDialogBuilder(this)
                .setTitle(R.string.preferences_restore)
                .setMessage(R.string.preferences_restore_confirm)
                .setPositiveButton(R.string.preferences_restore_action) { _, _ ->
                    dialog.dismiss()
                    restoreDefaults()
                }
                .setNegativeButton(R.string.preferences_cancel, null)
                .show()
        }
        dialog.setOnShowListener {
            dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                val settings = with(dialogBinding) {
                    AppSettings(
                        defaultFormat = formats.getOrElse(prefFormat.selectedItemPosition) { "auto" },
                        defaultProfile = AppSettings.PROFILES.getOrElse(prefProfile.selectedItemPosition) { "baseline" },
                        tsaEnabled = prefTsaEnabled.isChecked,
                        tsaUrl = prefTsaUrl.text?.toString()?.trim().orEmpty(),
                        outputName = OutputNames.POLICIES.getOrElse(prefOutputName.selectedItemPosition) { OutputNames.SUFFIX },
                        theme = AppSettings.THEMES.getOrElse(prefTheme.selectedItemPosition) { AppSettings.THEME_SYSTEM },
                        sessionTimeoutMinutes = AppSettings.TIMEOUTS.getOrElse(prefSessionTimeout.selectedItemPosition) {
                            AppSettings.DEFAULT_TIMEOUT
                        },
                    )
                }
                if (viewModel.savePreferences(settings)) {
                    dialogBinding.prefTsaUrlLayout.error = null
                    dialog.dismiss()
                    selectDefaultFormat()
                    AppCompatDelegate.setDefaultNightMode(AppSettings.nightMode(settings.theme))
                } else {
                    dialogBinding.prefTsaUrlLayout.error = getString(R.string.error_tsa_configuration)
                }
            }
        }
        dialog.show()
    }

    /** Restaura preferencias y sello; los certificados y documentos no cambian. */
    private fun restoreDefaults() {
        viewModel.restoreDefaultPreferences()
        sealPreferences.clear()
        sealImageFile.delete()
        sealSettings = sealPreferences.load()
        sealPageInfo = null
        selectDefaultFormat()
        render(viewModel.state.value)
        AppCompatDelegate.setDefaultNightMode(AppSettings.nightMode(AppSettings.THEME_SYSTEM))
    }

    private fun showDiagnostics() {
        viewModel.loadDiagnostics()
        val dialogBinding = DialogDiagnosticsBinding.inflate(layoutInflater)
        dialogBinding.diagnosticsProbeTsa.setOnClickListener { viewModel.probeTsa() }
        dialogBinding.diagnosticsCopy.setOnClickListener { copyDiagnostics() }
        diagnosticsDialog = dialogBinding
        MaterialAlertDialogBuilder(this)
            .setTitle(R.string.diagnostics_title)
            .setView(dialogBinding.root)
            .setPositiveButton(R.string.help_close, null)
            .setOnDismissListener { diagnosticsDialog = null }
            .show()
        renderDiagnostics(viewModel.state.value)
    }

    private fun diagnosticsText(state: MainUiState): String {
        val facts = AppFacts(
            appVersion = BuildConfig.VERSION_NAME,
            sourceCommit = BuildConfig.SOURCE_COMMIT,
            coreSha256 = BuildConfig.CORE_EXPECTED_SHA256,
            androidRelease = android.os.Build.VERSION.RELEASE,
            sdk = android.os.Build.VERSION.SDK_INT,
            language = resources.configuration.locales[0].toLanguageTag(),
            deviceTimeIso = java.time.OffsetDateTime.now().truncatedTo(java.time.temporal.ChronoUnit.SECONDS).toString(),
            timeZone = TimeZone.getDefault().id,
        )
        return DiagnosticsText.lines(facts, state.diagnostics, state.tsaUrl, state.tsaProbe).resolve(this)
    }

    private fun renderDiagnostics(state: MainUiState) {
        val dialogBinding = diagnosticsDialog ?: return
        dialogBinding.diagnosticsReport.text = diagnosticsText(state)
        dialogBinding.diagnosticsProbeTsa.isEnabled = state.canProbeTsa
        dialogBinding.diagnosticsProbeTsa.visibility = if (PlatformServicesVisibility.tsa(state)) View.VISIBLE else View.GONE
    }

    /** Copia el informe sin datos personales; Android 13+ muestra su propio aviso. */
    private fun copyDiagnostics() {
        val text = getString(R.string.diag_report_title) + "\n\n" + diagnosticsText(viewModel.state.value)
        val clipboard = getSystemService(android.content.ClipboardManager::class.java) ?: return
        clipboard.setPrimaryClip(android.content.ClipData.newPlainText(getString(R.string.diag_report_title), text))
        if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.TIRAMISU) {
            showMessage(R.string.diag_copied)
        }
    }

    private fun renderUpdateCheck(state: MainUiState) {
        val check = state.updateCheck ?: run { shownUpdateCheck = null; return }
        if (check === shownUpdateCheck) return
        shownUpdateCheck = check
        MaterialAlertDialogBuilder(this)
            .setTitle(R.string.update_title)
            .setMessage(UpdateText.lines(check).resolve(this))
            .setPositiveButton(R.string.help_close, null)
            .apply {
                if (check.status == "newer" || check.status == "current" || check.status == "not_comparable") {
                    setNeutralButton(R.string.update_open_release) { _, _ -> viewModel.openRelease() }
                }
            }
            .setOnDismissListener { viewModel.dismissUpdateCheck() }
            .show()
    }

    /** Solo se abre una publicación del repositorio oficial por HTTPS; nunca se descarga nada. */
    private fun openRelease(url: String) {
        if (!AppLinks.isOfficialRelease(url)) return
        try {
            startActivity(Intent(Intent.ACTION_VIEW, url.toUri()).addCategory(Intent.CATEGORY_BROWSABLE))
        } catch (_: android.content.ActivityNotFoundException) {
            viewModel.reportPickerError()
        }
    }

    /** Campo con contorno y márgenes del diálogo; el PIN se puede mostrar u ocultar. */
    private fun secretInputLayout(hint: Int, field: TextInputEditText, password: Boolean): TextInputLayout {
        val layout = TextInputLayout(this, null, com.google.android.material.R.attr.textInputOutlinedStyle)
        layout.hint = getString(hint)
        if (password) layout.endIconMode = TextInputLayout.END_ICON_PASSWORD_TOGGLE
        layout.addView(field)
        val padding = (24 * resources.displayMetrics.density).toInt()
        android.widget.FrameLayout(this).apply {
            setPadding(padding, padding / 3, padding, 0)
            addView(layout)
        }
        return layout
    }

    /** Acceso de las pantallas de la cuarta oleada al DNIe de la sesión. */
    private val dnieAccess = object : Wave4Screen.DnieAccess {
        override fun withPin(hold: Boolean, action: () -> Unit) {
            if (usesDnie()) showDniePinDialog(hold, action) else action()
        }

        override fun endOperation() {
            dnieSession?.endOperation()
        }

        override fun takeError(): Pair<Throwable, Int>? {
            val session = dnieSession ?: return null
            return session.consumeSigningError()?.let { it to session.retriesLeft() }
        }
    }

    /** Sello del lote: el guardado en «Firma visible», adaptado a cada PDF. */
    private fun batchSealPlanner(): PdfBatchSealPlanner? {
        if (!viewModel.state.value.wave4.batchSeal) return null
        return try {
            val image = if (sealSettings.logo == "custom" && sealImageFile.isFile && sealImageFile.length() in 1..(2L shl 20)) {
                val bytes = sealImageFile.readBytes()
                try { Base64.getEncoder().encodeToString(bytes) } finally { bytes.fill(0) }
            } else null
            PdfBatchSealPlanner(File(noBackupFilesDir, BatchSeal.WORK_DIRECTORY), sealSettings, image)
        } catch (_: Exception) {
            null
        }
    }

    /** Ayuda por apartados: cada uno en pasos cortos, sin un bloque largo que desplazar. */
    private fun showHelp() {
        val topics = listOf(
            R.string.help_topic_sign to R.string.help_sign_content,
            R.string.help_topic_dnie to R.string.help_dnie_content,
            R.string.help_topic_verify to R.string.help_verify_content,
            R.string.help_topic_tools to R.string.help_tools_content,
            R.string.help_topic_privacy to R.string.help_privacy_content,
        )
        MaterialAlertDialogBuilder(this)
            .setTitle(R.string.help_title)
            .setItems(topics.map { getString(it.first) }.toTypedArray()) { _, index ->
                val (title, content) = topics[index]
                MaterialAlertDialogBuilder(this)
                    .setTitle(title)
                    .setMessage(content)
                    .setPositiveButton(R.string.help_close, null)
                    .setNeutralButton(R.string.menu_help) { _, _ -> showHelp() }
                    .show()
            }
            .setPositiveButton(R.string.help_close, null)
            .show()
    }
}
