// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import android.graphics.Bitmap
import androidx.core.graphics.createBitmap
import android.graphics.BitmapFactory
import android.graphics.Color
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.CheckBox
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.SeekBar
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.isGone
import androidx.lifecycle.lifecycleScope
import com.google.android.material.button.MaterialButton
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.textfield.TextInputEditText
import com.google.android.material.textfield.TextInputLayout
import androidx.core.widget.doAfterTextChanged
import io.github.aavidad.grxfirma.android.ui.EngineKeys
import io.github.aavidad.grxfirma.android.ui.resolve
import io.github.aavidad.grxfirma.android.ui.toUserText
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.files.ContentRepository
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.ui.MainViewModel
import java.io.File
import java.util.Base64
import io.github.aavidad.grxfirma.android.ui.DropdownField
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** El PDF temporal se crea dentro de cacheDir y se elimina al cerrar el editor. */
class SealEditorDialog(
    private val activity: AppCompatActivity,
    private val viewModel: MainViewModel,
    private val document: SelectedFile,
    initial: SealSettings,
    private val imageFile: File,
    private val chooseImage: () -> Unit,
    private val onSaved: (SealSettings, Int, Int, Int) -> Unit,
    private val onClosed: () -> Unit,
    /** Avisos que bloquean la apertura: la actividad los muestra en un Snackbar. */
    private val onError: (Int) -> Unit = {},
) {
    private var settings = initial.copy(enabled = true)
    private var tempPdf: File? = null
    private var pages: List<Pair<Int, Int>> = emptyList()
    private var previewJob: Job? = null
    private var dialog: androidx.appcompat.app.AlertDialog? = null
    private lateinit var canvas: SealCanvasView
    private lateinit var pageLabel: TextView
    private lateinit var previewStatus: TextView
    private var previewValid = false
    private lateinit var qrAddress: EditText
    private lateinit var qrAddressLayout: TextInputLayout
    private lateinit var navigation: LinearLayout
    private lateinit var qrCheck: CheckBox
    private lateinit var logoDropdown: DropdownField
    private lateinit var chooseImageButton: View
    private lateinit var pageToggle: MaterialButton
    private lateinit var pagesSummary: TextView
    private var csvJob: Job? = null
    private var csvValid = false
    private var csvStatus: TextView? = null
    private var csvCodeLayout: TextInputLayout? = null
    private var csvUrlLayout: TextInputLayout? = null
    private var csvTextLayout: TextInputLayout? = null

    fun show() {
        activity.lifecycleScope.launch {
            try {
                val info = withContext(Dispatchers.IO) { preparePdf() }
                pages = info
                settings = settings.copy(
                    page = settings.page.coerceIn(1, pages.size),
                    allPages = settings.allPages && pages.size <= SealSettings.MAX_PLACEMENTS,
                    placements = settings.placements.filterKeys { it in 1..pages.size },
                ).let { if (it.perPage) it.loadPage(it.page) else it }
                buildDialog()
                showPage()
            } catch (_: Exception) {
                tempPdf?.delete()
                onClosed()
                onError(R.string.seal_pdf_error)
            }
        }
    }

    fun customImageSelected() {
        settings = settings.copy(logo = "custom")
        if (::logoDropdown.isInitialized) logoDropdown.select(2)
        if (::chooseImageButton.isInitialized) chooseImageButton.visibility = View.VISIBLE
        refreshPreview()
    }

    private fun preparePdf(): List<Pair<Int, Int>> {
        val loaded = ContentRepository(activity.contentResolver).loadDocument(document)
        val file = File.createTempFile("seal-page-", ".pdf", activity.cacheDir)
        tempPdf = file
        try {
            file.outputStream().use { it.write(loaded.bytes) }
        } finally {
            loaded.bytes.fill(0)
        }
        return withRenderer(file) { renderer ->
            require(renderer.pageCount in 1..2048)
            (0 until renderer.pageCount).map { index ->
                renderer.openPage(index).use { it.width to it.height }
            }
        }
    }

    private fun buildDialog() {
        val scroll = android.widget.ScrollView(activity)
        val column = LinearLayout(activity).apply {
            orientation = LinearLayout.VERTICAL
            val pad = dp(16)
            setPadding(pad, pad, pad, pad)
        }
        scroll.addView(column)
        val instructions = label(R.string.seal_edit_hint)
        column.addView(instructions)
        canvas = SealCanvasView(activity).apply {
            settings = this@SealEditorDialog.settings
            onEdited = { edited ->
                // En «varias páginas», mover el sello lo coloca en la página actual.
                this@SealEditorDialog.settings = if (edited.perPage) edited.withPagePlacement(edited.page) else edited
                updatePageControls()
                refreshPreview()
            }
        }
        // La página no ocupa toda la ventana: debajo deben asomar los botones de ajuste.
        // Primero un 40 % de la pantalla; al mostrarse se ajusta al alto real del diálogo.
        canvas.maxPageHeight = (activity.resources.displayMetrics.heightPixels * 0.40f).toInt()
        column.addView(canvas, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT)
            .apply { gravity = android.view.Gravity.CENTER_HORIZONTAL })
        previewStatus = label(R.string.seal_preview_loading).apply { accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE }
        column.addView(previewStatus)
        pageLabel = label(R.string.seal_page_placeholder).apply { textAlignment = View.TEXT_ALIGNMENT_CENTER }
        column.addView(pageLabel)
        // Con una sola página no hay a dónde ir: la navegación no se muestra.
        navigation = row()
        navigation.addView(button(R.string.seal_previous, outlined = true) { changePage(-1) }, weighted())
        navigation.addView(button(R.string.seal_next, outlined = true) { changePage(1) }, weighted())
        navigation.visibility = if (pages.size > 1) View.VISIBLE else View.GONE
        column.addView(navigation)

        // Iconos con su descripción: no hay palabras que se corten con letra grande.
        val controlsLabel = label(R.string.seal_accessible_controls)
        column.addView(controlsLabel)
        val move = row()
        move.addView(iconButton(R.drawable.ic_seal_left, R.string.seal_move_left) { adjust(dx = -0.02f) }, weighted())
        move.addView(iconButton(R.drawable.ic_seal_right, R.string.seal_move_right) { adjust(dx = 0.02f) }, weighted())
        move.addView(iconButton(R.drawable.ic_seal_up, R.string.seal_move_up) { adjust(dy = 0.02f) }, weighted())
        move.addView(iconButton(R.drawable.ic_seal_down, R.string.seal_move_down) { adjust(dy = -0.02f) }, weighted())
        column.addView(move)
        val sizeAndRotation = row()
        sizeAndRotation.addView(iconButton(R.drawable.ic_seal_smaller, R.string.seal_smaller_desc) { adjust(dw = -0.02f, dh = -0.01f) }, weighted())
        sizeAndRotation.addView(iconButton(R.drawable.ic_seal_larger, R.string.seal_larger_desc) { adjust(dw = 0.02f, dh = 0.01f) }, weighted())
        sizeAndRotation.addView(iconButton(R.drawable.ic_seal_rotate_left, R.string.seal_rotate_left_desc) { adjust(angle = -15) }, weighted())
        sizeAndRotation.addView(iconButton(R.drawable.ic_seal_rotate_right, R.string.seal_rotate_right_desc) { adjust(angle = 15) }, weighted())
        column.addView(sizeAndRotation)

        column.addView(label(R.string.seal_opacity))
        column.addView(SeekBar(activity).apply {
            max = 100
            progress = settings.opacity
            contentDescription = activity.getString(R.string.seal_opacity)
            setOnSeekBarChangeListener(object : SeekBar.OnSeekBarChangeListener {
                override fun onProgressChanged(seekBar: SeekBar?, progress: Int, fromUser: Boolean) {
                    if (fromUser) { settings = settings.copy(opacity = progress); refreshPreview() }
                }
                override fun onStartTrackingTouch(seekBar: SeekBar?) = Unit
                override fun onStopTrackingTouch(seekBar: SeekBar?) = Unit
            })
        }, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(48)))

        qrCheck = CheckBox(activity).apply {
            setText(R.string.seal_qr)
            minHeight = dp(48)
            isChecked = settings.qrEnabled
            setOnCheckedChangeListener { _, checked ->
                settings = settings.copy(qrEnabled = checked)
                qrAddressLayout.visibility = if (checked) View.VISIBLE else View.GONE
                refreshPreview()
            }
        }
        column.addView(qrCheck)
        // Campo con contorno y ejemplo, como los demás; su error se ve en el propio campo.
        qrAddressLayout = TextInputLayout(activity, null, com.google.android.material.R.attr.textInputOutlinedStyle).apply {
            hint = activity.getString(R.string.seal_qr_address)
            placeholderText = activity.getString(R.string.seal_qr_address_example)
            visibility = if (settings.qrEnabled) View.VISIBLE else View.GONE
            setPadding(0, dp(8), 0, 0)
        }
        qrAddress = TextInputEditText(qrAddressLayout.context).apply {
            setSingleLine(true)
            minHeight = dp(48)
            importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_NO
            inputType = android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_VARIATION_URI
            setText(settings.qrAddress)
            addTextChangedListener(object : android.text.TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                    settings = settings.copy(qrAddress = s.toString())
                    refreshPreview()
                }
                override fun afterTextChanged(s: android.text.Editable?) = Unit
            })
        }
        qrAddressLayout.addView(qrAddress, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        column.addView(qrAddressLayout)

        val logoField = dropdown(R.string.seal_style, listOf(R.string.seal_logo_none, R.string.seal_logo_institutional,
            R.string.seal_logo_custom), when (settings.logo) { "institutional" -> 1; "custom" -> 2; else -> 0 }) { position ->
            settings = settings.copy(logo = listOf("none", "institutional", "custom")[position])
            chooseImageButton.visibility = if (settings.logo == "custom") View.VISIBLE else View.GONE
            refreshPreview()
        }
        logoDropdown = logoField.second
        column.addView(logoField.first)
        // Acción secundaria: solo tiene sentido con «Imagen propia».
        chooseImageButton = button(R.string.seal_choose_image, outlined = true) { chooseImage() }
        chooseImageButton.visibility = if (settings.logo == "custom") View.VISIBLE else View.GONE
        column.addView(chooseImageButton)
        column.addView(CheckBox(activity).apply {
            setText(R.string.seal_show_text)
            minHeight = dp(48)
            isChecked = settings.keepText
            setOnCheckedChangeListener { _, checked ->
                settings = settings.copy(keepText = checked)
                refreshPreview()
            }
        })
        column.addView(dropdown(R.string.seal_text_color, listOf(R.string.seal_color_black, R.string.seal_color_blue,
            R.string.seal_color_gray), when (settings.textColor) { "blue" -> 1; "darkgray" -> 2; else -> 0 }) { position ->
            settings = settings.copy(textColor = listOf("black", "blue", "darkgray")[position])
            refreshPreview()
        }.first)
        lateinit var pagesMode: DropdownField
        val pagesField = dropdown(R.string.seal_pages_mode, listOf(R.string.seal_one_page, R.string.seal_all_pages,
            R.string.seal_pages_custom), when { settings.perPage -> 2; settings.allPages -> 1; else -> 0 }) { position ->
            if (position == 1 && pages.size > SealSettings.MAX_PLACEMENTS) {
                pagesMode.select(0)
                return@dropdown
            }
            settings = when (position) {
                1 -> settings.copy(allPages = true, perPage = false)
                2 -> settings.copy(allPages = false, perPage = true)
                    .let { if (it.placements.isEmpty()) it.withPagePlacement(it.page) else it.loadPage(it.page) }
                else -> settings.copy(allPages = false, perPage = false)
            }
            updatePageControls()
            refreshPreview()
        }
        pagesMode = pagesField.second
        column.addView(pagesField.first)
        pageToggle = button(R.string.seal_page_add) { togglePage() }
        column.addView(pageToggle)
        pagesSummary = label(R.string.seal_pages_none).apply { accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE }
        column.addView(pagesSummary)
        updatePageControls()
        if (viewModel.state.value.csvLegendAvailable) buildCsvSection(column)

        dialog = MaterialAlertDialogBuilder(activity)
            .setTitle(R.string.seal_visible)
            .setView(scroll)
            .setNegativeButton(android.R.string.cancel, null)
            .setPositiveButton(R.string.seal_apply, null)
            .create().also { actual ->
                actual.setOnDismissListener {
                    previewJob?.cancel()
                    csvJob?.cancel()
                    canvas.pageBitmap?.recycle()
                    canvas.sealBitmap?.recycle()
                    tempPdf?.delete()
                    dialog = null
                    onClosed()
                }
                actual.show()
                actual.window?.setLayout(WindowManager.LayoutParams.MATCH_PARENT, WindowManager.LayoutParams.MATCH_PARENT)
                // Al abrir deben verse la página, su estado y la primera fila de botones.
                scroll.post { fitCanvasToDialog(scroll, column, listOf(instructions, previewStatus, pageLabel, navigation, controlsLabel, move)) }
                actual.getButton(androidx.appcompat.app.AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                    saveIfValid(actual)
                }
            }
    }

    /** Limita el lienzo al hueco que dejan en el diálogo los textos y la primera fila de botones. */
    private fun fitCanvasToDialog(scroll: View, column: View, around: List<View>) {
        if (scroll.height <= 0) return
        val reserved = column.paddingTop + around.sumOf { if (it.isGone) 0 else it.height } + dp(16)
        val available = scroll.height - reserved
        val limit = (activity.resources.displayMetrics.heightPixels * 0.55f).toInt()
        if (available >= dp(120)) canvas.maxPageHeight = available.coerceAtMost(limit)
    }

    private fun saveIfValid(actual: androidx.appcompat.app.AlertDialog) {
        settingsProblem()?.let { problem ->
            showProblem(problem)
            if (problem != R.string.seal_image_required) qrAddress.requestFocus()
            return
        }
        if (settings.csvEnabled && !csvValid) {
            showProblem(R.string.seal_invalid_settings)
            return
        }
        try {
            val (w, h) = pages[settings.page - 1]
            require(previewValid)
            val image = imageBase64()
            settings.options(pages.size, w, h, image)
            // Cada página con sello comprueba su propia posición y su tamaño de página.
            require(settings.placementList(pages.size).all { (number, placement) ->
                val (pw, ph) = pages[number - 1]
                val (bw, bh) = SealGeometry.rotatedBounds(placement.rect, placement.rotation, pw.toFloat() / ph)
                bw <= 1f && bh <= 1f
            })
            onSaved(settings, pages.size, w, h)
            actual.dismiss()
        } catch (_: Exception) {
            showProblem(R.string.seal_invalid_settings)
        }
    }

    private fun changePage(delta: Int) {
        val next = (settings.page + delta).coerceIn(1, pages.size)
        if (next != settings.page) {
            settings = if (settings.perPage) settings.loadPage(next) else settings.copy(page = next)
            updatePageControls()
            showPage()
        }
    }

    private fun showPage() {
        pageLabel.text = activity.getString(R.string.seal_page_number, settings.page, pages.size)
        val file = tempPdf ?: return
        val pageNumber = settings.page
        activity.lifecycleScope.launch {
            try {
                val bitmap = withContext(Dispatchers.IO) {
                    withRenderer(file) { renderer ->
                        renderer.openPage(pageNumber - 1).use { page ->
                            val scale = minOf(1400f / page.width, 1400f / page.height, 2f)
                            val bitmap = createBitmap(
                                (page.width * scale).toInt().coerceAtLeast(1),
                                (page.height * scale).toInt().coerceAtLeast(1),
                            )
                            bitmap.eraseColor(Color.WHITE)
                            page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                            bitmap
                        }
                    }
                }
                if (dialog == null || settings.page != pageNumber) bitmap.recycle()
                else {
                    canvas.pageBitmap?.recycle()
                    canvas.pageBitmap = bitmap
                    canvas.settings = settings
                    refreshPreview()
                }
            } catch (_: Exception) {
                showProblem(R.string.seal_pdf_error)
            }
        }
    }

    /**
     * Lo que falta para poder generar el sello, dicho en concreto. La dirección
     * del QR se marca además en su campo.
     */
    private fun settingsProblem(): Int? {
        val address = settings.qrAddress.trim()
        val problem = when {
            settings.qrEnabled && address.isEmpty() -> R.string.seal_qr_address_required
            settings.qrEnabled && runCatching { normalizedHttps(address) }.isFailure -> R.string.seal_qr_address_https
            settings.logo == "custom" && !(imageFile.isFile && imageFile.length() in 1..(2L shl 20)) -> R.string.seal_image_required
            else -> null
        }
        if (::qrAddressLayout.isInitialized) {
            qrAddressLayout.error = if (problem == R.string.seal_qr_address_required || problem == R.string.seal_qr_address_https)
                activity.getString(problem) else null
        }
        return problem
    }

    fun refreshPreview() {
        if (dialog == null || pages.isEmpty()) return
        canvas.settings = settings
        previewValid = false
        settingsProblem()?.let { problem ->
            previewJob?.cancel()
            canvas.sealBitmap = null
            showProblem(problem, announce = false)
            return
        }
        previewStatus.setText(R.string.seal_preview_loading)
        previewStatus.setTextColor(androidx.core.content.ContextCompat.getColor(activity, R.color.on_surface))
        previewJob?.cancel()
        previewJob = activity.lifecycleScope.launch {
            delay(300)
            try {
                val (w, h) = pages[settings.page - 1]
                val image = imageBase64()
                // La imagen del sello no depende de las páginas elegidas ni de la leyenda CSV.
                val options = settings.copy(rotation = 0, perPage = false, allPages = false, csvEnabled = false)
                    .options(pages.size, w, h, image)
                val png = viewModel.previewSeal(options)
                val bitmap = withContext(Dispatchers.Default) { BitmapFactory.decodeByteArray(png, 0, png.size) }
                png.fill(0)
                if (dialog == null) bitmap?.recycle() else {
                    canvas.sealBitmap?.recycle()
                    canvas.sealBitmap = bitmap
                    previewValid = bitmap != null
                    if (previewValid) previewStatus.setText(R.string.seal_preview_ready)
                    else showProblem(R.string.seal_preview_error, announce = false)
                }
            } catch (error: kotlinx.coroutines.CancellationException) {
                // Otra vista previa ha sustituido a esta: no es un error.
                throw error
            } catch (_: Exception) {
                canvas.sealBitmap = null
                showProblem(R.string.seal_preview_error, announce = false)
            }
        }
    }

    private fun adjust(dx: Float = 0f, dy: Float = 0f, dw: Float = 0f, dh: Float = 0f, angle: Int = 0) {
        val rect = settings.rect
        val next = settings.copy(
            rect = rect.copy(x = rect.x + dx, y = rect.y + dy,
                w = (rect.w + dw).coerceAtLeast(0.08f), h = (rect.h + dh).coerceAtLeast(0.04f)),
            rotation = SealGeometry.snap(settings.rotation + angle),
        )
        try {
            val (w, h) = pages[settings.page - 1]
            val fitted = next.copy(rect = SealGeometry.fit(next.rect, next.rotation, w.toFloat() / h))
            if (fitted.rect.valid()) {
                settings = if (fitted.perPage) fitted.withPagePlacement(fitted.page) else fitted
                updatePageControls()
                refreshPreview()
            }
        } catch (_: IllegalArgumentException) { /* Sin cambio. */ }
    }

    private fun togglePage() {
        settings = if (settings.page in settings.placements) settings.withoutPage(settings.page)
        else settings.withPagePlacement(settings.page)
        updatePageControls()
    }

    private fun updatePageControls() {
        if (!::pageToggle.isInitialized) return
        val custom = settings.perPage
        pageToggle.visibility = if (custom) View.VISIBLE else View.GONE
        pagesSummary.visibility = pageToggle.visibility
        val onPage = settings.page in settings.placements
        pageToggle.setText(if (onPage) R.string.seal_page_remove else R.string.seal_page_add)
        pageToggle.contentDescription = pageToggle.text
        pagesSummary.text = if (settings.placements.isEmpty()) activity.getString(R.string.seal_pages_none)
        else activity.getString(R.string.seal_pages_selected, settings.placements.keys.sorted().joinToString(", "))
        if (::canvas.isInitialized) canvas.sealOnPage = !custom || onPage
    }

    /** Leyenda CSV: el motor valida y normaliza la URL antes de aceptar el editor. */
    private fun buildCsvSection(column: LinearLayout) {
        val group = LinearLayout(activity).apply { orientation = LinearLayout.VERTICAL }
        val enable = CheckBox(activity).apply {
            setText(R.string.csv_enable)
            minHeight = dp(48)
            isChecked = settings.csvEnabled
        }
        column.addView(enable)
        group.addView(label(R.string.csv_notice))
        group.addView(label(R.string.csv_placement_note))
        csvCodeLayout = textField(group, R.string.csv_code, settings.csvCode, SealSettings.MAX_CSV_CODE,
            android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS) {
            settings = settings.copy(csvCode = it); validateCsv()
        }
        csvUrlLayout = textField(group, R.string.csv_url, settings.csvUrl, SealSettings.MAX_CSV_URL,
            android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_VARIATION_URI) {
            settings = settings.copy(csvUrl = it); validateCsv()
        }
        csvTextLayout = textField(group, R.string.csv_text, settings.csvText, SealSettings.MAX_CSV_TEXT,
            android.text.InputType.TYPE_CLASS_TEXT) {
            settings = settings.copy(csvText = it); validateCsv()
        }
        group.addView(CheckBox(activity).apply {
            setText(R.string.csv_qr)
            minHeight = dp(48)
            isChecked = settings.csvQr
            setOnCheckedChangeListener { _, checked -> settings = settings.copy(csvQr = checked) }
        })
        csvStatus = label(R.string.csv_checking).apply {
            accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE
            setTextIsSelectable(true)
        }
        group.addView(csvStatus)
        group.visibility = if (settings.csvEnabled) View.VISIBLE else View.GONE
        enable.setOnCheckedChangeListener { _, checked ->
            settings = settings.copy(csvEnabled = checked)
            group.visibility = if (checked) View.VISIBLE else View.GONE
            validateCsv()
        }
        column.addView(group)
        if (settings.csvEnabled) validateCsv()
    }

    private fun textField(
        parent: LinearLayout, hint: Int, value: String, maximum: Int, type: Int, changed: (String) -> Unit,
    ): TextInputLayout {
        val layout = TextInputLayout(activity).apply { this.hint = activity.getString(hint) }
        val field = TextInputEditText(layout.context).apply {
            inputType = type
            setSingleLine(true)
            filters = arrayOf(android.text.InputFilter.LengthFilter(maximum))
            importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_NO
            minHeight = dp(48)
            setText(value)
            doAfterTextChanged { changed(it?.toString().orEmpty()) }
        }
        layout.addView(field)
        parent.addView(layout)
        return layout
    }

    private fun validateCsv() {
        csvValid = false
        csvJob?.cancel()
        listOf(csvCodeLayout, csvUrlLayout, csvTextLayout).forEach { it?.error = null }
        if (!settings.csvEnabled) {
            csvValid = true
            return
        }
        csvStatus?.setText(R.string.csv_checking)
        val snapshot = settings
        csvJob = activity.lifecycleScope.launch {
            delay(400)
            try {
                val legend = viewModel.csvLegend(snapshot.csvCode, snapshot.csvUrl, snapshot.csvText)
                if (dialog == null) return@launch
                csvValid = true
                csvStatus?.text = activity.getString(R.string.csv_preview, legend.url, legend.text)
            } catch (error: kotlinx.coroutines.CancellationException) {
                throw error
            } catch (error: Exception) {
                if (dialog == null) return@launch
                val key = (error as? io.github.aavidad.grxfirma.android.core.CoreContractException)?.message.orEmpty()
                val message = error.toUserText().resolve(activity)
                when (EngineKeys.csvField(key)) {
                    "code" -> csvCodeLayout?.error = message
                    "url" -> csvUrlLayout?.error = message
                    "text" -> csvTextLayout?.error = message
                }
                csvStatus?.text = message
            }
        }
    }

    private fun imageBase64(): String? = if (settings.logo == "custom" && imageFile.isFile && imageFile.length() in 1..(2L shl 20)) {
        val bytes = imageFile.readBytes()
        try { Base64.getEncoder().encodeToString(bytes) } finally { bytes.fill(0) }
    } else null

    private fun row() = LinearLayout(activity).apply { orientation = LinearLayout.HORIZONTAL }
    // Alto según el contenido: con letra grande el texto no se corta.
    private fun weighted() = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)

    /** Campo desplegable con su etiqueta, como en el resto de la app. */
    private fun dropdown(label: Int, options: List<Int>, selected: Int, onSelected: (Int) -> Unit): Pair<TextInputLayout, DropdownField> {
        val layout = TextInputLayout(activity, null, com.google.android.material.R.attr.textInputOutlinedExposedDropdownMenuStyle)
        layout.hint = activity.getString(label)
        val field = DropdownField(layout.context).apply {
            minHeight = dp(48)
            setItems(options.map(activity::getString))
            select(selected)
            onItemSelected = onSelected
        }
        layout.addView(field, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        layout.setPadding(0, dp(8), 0, 0)
        return layout to field
    }

    /** Problema dentro del editor: se muestra y se anuncia en la línea de estado. */
    private fun showProblem(message: Int, announce: Boolean = true) {
        if (!::previewStatus.isInitialized) return onError(message)
        previewStatus.setText(message)
        previewStatus.setTextColor(androidx.core.content.ContextCompat.getColor(activity, R.color.error))
        if (announce) previewStatus.announceForAccessibility(previewStatus.text)
    }
    private fun label(id: Int) = TextView(activity).apply {
        setText(id)
        textSize = 16f
        setPadding(0, dp(8), 0, dp(8))
        ViewCompat.setAccessibilityHeading(this, id == R.string.seal_accessible_controls)
    }
    /** [outlined]: botón secundario, para que no compita con «Aplicar sello». */
    private fun button(id: Int, outlined: Boolean = false, action: () -> Unit) =
        (if (outlined) MaterialButton(activity, null, com.google.android.material.R.attr.materialButtonOutlinedStyle)
        else MaterialButton(activity)).apply {
            setText(id)
            minHeight = dp(48)
            setOnClickListener { action() }
        }

    /** Botón de solo icono: TalkBack lee [description] y una pulsación larga la muestra. */
    private fun iconButton(icon: Int, description: Int, action: () -> Unit) =
        MaterialButton(activity, null, com.google.android.material.R.attr.materialButtonOutlinedStyle).apply {
            setIconResource(icon)
            iconGravity = MaterialButton.ICON_GRAVITY_TEXT_START
            iconPadding = 0
            minHeight = dp(48)
            minWidth = dp(48)
            contentDescription = activity.getString(description)
            androidx.appcompat.widget.TooltipCompat.setTooltipText(this, contentDescription)
            setOnClickListener { action() }
        }
    private fun dp(value: Int) = (value * activity.resources.displayMetrics.density).toInt()

    private inline fun <T> withRenderer(file: File, block: (PdfRenderer) -> T): T =
        ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { descriptor ->
            PdfRenderer(descriptor).use(block)
        }
}
