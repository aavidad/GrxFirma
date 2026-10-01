// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.seal

import android.graphics.Bitmap
import androidx.core.graphics.createBitmap
import android.graphics.BitmapFactory
import android.graphics.Color
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.ArrayAdapter
import android.widget.CheckBox
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.SeekBar
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.lifecycle.lifecycleScope
import com.google.android.material.button.MaterialButton
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.files.ContentRepository
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.ui.MainViewModel
import java.io.File
import java.util.Base64
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
    private lateinit var qrCheck: CheckBox
    private lateinit var allPages: CheckBox
    private lateinit var logoSpinner: Spinner

    fun show() {
        activity.lifecycleScope.launch {
            try {
                val info = withContext(Dispatchers.IO) { preparePdf() }
                pages = info
                settings = settings.copy(
                    page = settings.page.coerceIn(1, pages.size),
                    allPages = settings.allPages && pages.size <= 128,
                )
                buildDialog()
                showPage()
            } catch (_: Exception) {
                tempPdf?.delete()
                onClosed()
                Toast.makeText(activity, R.string.seal_pdf_error, Toast.LENGTH_LONG).show()
            }
        }
    }

    fun customImageSelected() {
        settings = settings.copy(logo = "custom")
        if (::logoSpinner.isInitialized) logoSpinner.setSelection(2)
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
            onEdited = { this@SealEditorDialog.settings = it; refreshPreview() }
        }
        column.addView(canvas, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        previewStatus = label(R.string.seal_preview_loading).apply { accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE }
        column.addView(previewStatus)
        pageLabel = label(R.string.seal_page_placeholder).apply { textAlignment = View.TEXT_ALIGNMENT_CENTER }
        column.addView(pageLabel)
        val navigation = row()
        navigation.addView(button(R.string.seal_previous) { changePage(-1) }, weighted())
        navigation.addView(button(R.string.seal_next) { changePage(1) }, weighted())
        column.addView(navigation)

        column.addView(label(R.string.seal_accessible_controls))
        val move = row()
        move.addView(button(R.string.seal_left) { adjust(dx = -0.02f) }, weighted())
        move.addView(button(R.string.seal_right) { adjust(dx = 0.02f) }, weighted())
        move.addView(button(R.string.seal_up) { adjust(dy = 0.02f) }, weighted())
        move.addView(button(R.string.seal_down) { adjust(dy = -0.02f) }, weighted())
        column.addView(move)
        val sizeAndRotation = row()
        sizeAndRotation.addView(button(R.string.seal_smaller) { adjust(dw = -0.02f, dh = -0.01f) }, weighted())
        sizeAndRotation.addView(button(R.string.seal_larger) { adjust(dw = 0.02f, dh = 0.01f) }, weighted())
        sizeAndRotation.addView(button(R.string.seal_rotate_left) { adjust(angle = -15) }, weighted())
        sizeAndRotation.addView(button(R.string.seal_rotate_right) { adjust(angle = 15) }, weighted())
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
                qrAddress.visibility = if (checked) View.VISIBLE else View.GONE
                refreshPreview()
            }
        }
        column.addView(qrCheck)
        qrAddress = EditText(activity).apply {
            hint = activity.getString(R.string.seal_qr_address)
            contentDescription = activity.getString(R.string.seal_qr_address)
            setSingleLine(true)
            inputType = android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_VARIATION_URI
            setText(settings.qrAddress)
            visibility = if (settings.qrEnabled) View.VISIBLE else View.GONE
            addTextChangedListener(object : android.text.TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                    settings = settings.copy(qrAddress = s.toString())
                    refreshPreview()
                }
                override fun afterTextChanged(s: android.text.Editable?) = Unit
            })
        }
        column.addView(qrAddress)

        column.addView(label(R.string.seal_style))
        logoSpinner = Spinner(activity).apply {
            adapter = ArrayAdapter(activity, android.R.layout.simple_spinner_dropdown_item, listOf(
                activity.getString(R.string.seal_logo_none),
                activity.getString(R.string.seal_logo_institutional),
                activity.getString(R.string.seal_logo_custom),
            ))
            minimumHeight = dp(48)
            setSelection(when (settings.logo) { "institutional" -> 1; "custom" -> 2; else -> 0 })
            onItemSelectedListener = object : android.widget.AdapterView.OnItemSelectedListener {
                override fun onItemSelected(parent: android.widget.AdapterView<*>?, view: View?, position: Int, id: Long) {
                    settings = settings.copy(logo = listOf("none", "institutional", "custom")[position])
                    refreshPreview()
                }
                override fun onNothingSelected(parent: android.widget.AdapterView<*>?) = Unit
            }
        }
        column.addView(logoSpinner)
        column.addView(button(R.string.seal_choose_image) { chooseImage() })
        column.addView(CheckBox(activity).apply {
            setText(R.string.seal_show_text)
            minHeight = dp(48)
            isChecked = settings.keepText
            setOnCheckedChangeListener { _, checked ->
                settings = settings.copy(keepText = checked)
                refreshPreview()
            }
        })
        column.addView(label(R.string.seal_text_color))
        column.addView(Spinner(activity).apply {
            adapter = ArrayAdapter(activity, android.R.layout.simple_spinner_dropdown_item, listOf(
                activity.getString(R.string.seal_color_black),
                activity.getString(R.string.seal_color_blue),
                activity.getString(R.string.seal_color_gray),
            ))
            minimumHeight = dp(48)
            setSelection(when (settings.textColor) { "blue" -> 1; "darkgray" -> 2; else -> 0 })
            onItemSelectedListener = object : android.widget.AdapterView.OnItemSelectedListener {
                override fun onItemSelected(parent: android.widget.AdapterView<*>?, view: View?, position: Int, id: Long) {
                    settings = settings.copy(textColor = listOf("black", "blue", "darkgray")[position])
                    refreshPreview()
                }
                override fun onNothingSelected(parent: android.widget.AdapterView<*>?) = Unit
            }
        })
        allPages = CheckBox(activity).apply {
            setText(R.string.seal_all_pages)
            minHeight = dp(48)
            isChecked = settings.allPages
            isEnabled = pages.size <= 128
            setOnCheckedChangeListener { _, checked -> settings = settings.copy(allPages = checked) }
        }
        column.addView(allPages)
        column.addView(button(R.string.seal_one_page) { allPages.isChecked = false })

        dialog = MaterialAlertDialogBuilder(activity)
            .setTitle(R.string.seal_visible)
            .setView(scroll)
            .setNegativeButton(android.R.string.cancel, null)
            .setPositiveButton(R.string.seal_apply, null)
            .create().also { actual ->
                actual.setOnDismissListener {
                    previewJob?.cancel()
                    canvas.pageBitmap?.recycle()
                    canvas.sealBitmap?.recycle()
                    tempPdf?.delete()
                    dialog = null
                    onClosed()
                }
                actual.show()
                actual.window?.setLayout(WindowManager.LayoutParams.MATCH_PARENT, WindowManager.LayoutParams.MATCH_PARENT)
                actual.getButton(androidx.appcompat.app.AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                    saveIfValid(actual)
                }
            }
    }

    private fun saveIfValid(actual: androidx.appcompat.app.AlertDialog) {
        try {
            val (w, h) = pages[settings.page - 1]
            require(previewValid)
            val image = imageBase64()
            settings.options(pages.size, w, h, image)
            val targets = if (settings.allPages) pages else listOf(w to h)
            require(targets.all { (pw, ph) ->
                val (bw, bh) = SealGeometry.rotatedBounds(settings.rect, settings.rotation, pw.toFloat() / ph)
                bw <= 1f && bh <= 1f
            })
            onSaved(settings, pages.size, w, h)
            actual.dismiss()
        } catch (_: Exception) {
            Toast.makeText(activity, R.string.seal_invalid_settings, Toast.LENGTH_LONG).show()
        }
    }

    private fun changePage(delta: Int) {
        val next = (settings.page + delta).coerceIn(1, pages.size)
        if (next != settings.page) {
            settings = settings.copy(page = next)
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
                Toast.makeText(activity, R.string.seal_pdf_error, Toast.LENGTH_LONG).show()
            }
        }
    }

    fun refreshPreview() {
        if (dialog == null || pages.isEmpty()) return
        canvas.settings = settings
        previewValid = false
        previewStatus.setText(R.string.seal_preview_loading)
        previewJob?.cancel()
        previewJob = activity.lifecycleScope.launch {
            delay(300)
            try {
                val (w, h) = pages[settings.page - 1]
                val image = imageBase64()
                val options = settings.copy(rotation = 0).options(pages.size, w, h, image)
                val png = viewModel.previewSeal(options)
                val bitmap = withContext(Dispatchers.Default) { BitmapFactory.decodeByteArray(png, 0, png.size) }
                png.fill(0)
                if (dialog == null) bitmap?.recycle() else {
                    canvas.sealBitmap?.recycle()
                    canvas.sealBitmap = bitmap
                    previewValid = bitmap != null
                    previewStatus.setText(if (previewValid) R.string.seal_preview_ready else R.string.seal_preview_error)
                }
            } catch (_: Exception) {
                canvas.sealBitmap = null
                previewStatus.setText(R.string.seal_preview_error)
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
            if (fitted.rect.valid()) { settings = fitted; refreshPreview() }
        } catch (_: IllegalArgumentException) { /* Sin cambio. */ }
    }

    private fun imageBase64(): String? = if (settings.logo == "custom" && imageFile.isFile && imageFile.length() in 1..(2L shl 20)) {
        val bytes = imageFile.readBytes()
        try { Base64.getEncoder().encodeToString(bytes) } finally { bytes.fill(0) }
    } else null

    private fun row() = LinearLayout(activity).apply { orientation = LinearLayout.HORIZONTAL }
    private fun weighted() = LinearLayout.LayoutParams(0, dp(48), 1f)
    private fun label(id: Int) = TextView(activity).apply {
        setText(id)
        textSize = 16f
        setPadding(0, dp(8), 0, dp(8))
        ViewCompat.setAccessibilityHeading(this, id == R.string.seal_accessible_controls)
    }
    private fun button(id: Int, action: () -> Unit) = MaterialButton(activity).apply {
        setText(id)
        minHeight = dp(48)
        contentDescription = when (id) {
            R.string.seal_left -> activity.getString(R.string.seal_move_left)
            R.string.seal_right -> activity.getString(R.string.seal_move_right)
            R.string.seal_up -> activity.getString(R.string.seal_move_up)
            R.string.seal_down -> activity.getString(R.string.seal_move_down)
            else -> activity.getString(id)
        }
        setOnClickListener { action() }
    }
    private fun dp(value: Int) = (value * activity.resources.displayMetrics.density).toInt()

    private inline fun <T> withRenderer(file: File, block: (PdfRenderer) -> T): T =
        ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { descriptor ->
            PdfRenderer(descriptor).use(block)
        }
}
