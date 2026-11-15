// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.content.res.ColorStateList
import android.os.Build
import android.view.View
import android.view.ViewGroup
import android.view.Window
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import androidx.core.content.ContextCompat
import androidx.core.graphics.ColorUtils
import androidx.core.view.ViewCompat
import com.google.android.material.button.MaterialButton
import com.google.android.material.checkbox.MaterialCheckBox
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.UnsafeExceptionState

/**
 * Título que anuncia TalkBack al abrir la ventana. Desde Android 9,
 * Window.setTitle lo guarda en accessibilityTitle; antes TalkBack leía el
 * título de los parámetros de la ventana, que por defecto es el de la actividad.
 */
internal fun Window.setAccessibleTitle(value: CharSequence) {
    setTitle(value)
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.P) attributes = attributes.apply { title = value }
}

private fun Context.dp(value: Int) = (value * resources.displayMetrics.density + 0.5f).toInt()
private fun column(context: Context) = LinearLayout(context).apply {
    orientation = LinearLayout.VERTICAL
}
private fun paragraph(context: Context, value: CharSequence, heading: Boolean = false) = TextView(context).apply {
    text = value
    setTextAppearance(if (heading) com.google.android.material.R.style.TextAppearance_Material3_TitleLarge
        else com.google.android.material.R.style.TextAppearance_Material3_BodyLarge)
    setPadding(0, context.dp(8), 0, context.dp(8))
    ViewCompat.setAccessibilityHeading(this, heading)
}
private fun MaterialButton.touchTarget(context: Context) = apply {
    insetTop = 0
    insetBottom = 0
    minHeight = context.dp(48)
    minimumHeight = context.dp(48)
    minWidth = context.dp(48)
    // Material usa TextView: el texto crece en sp y puede ocupar varias líneas.
    isSingleLine = false
}
private fun action(context: Context, value: String) = MaterialButton(context).touchTarget(context).apply {
    text = value
    layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
}
/** Acción secundaria del diálogo: borde o texto, nunca el relleno de la principal. */
private fun secondary(context: Context, value: String, outlined: Boolean) = MaterialButton(context, null,
    if (outlined) com.google.android.material.R.attr.materialButtonOutlinedStyle
    else androidx.appcompat.R.attr.borderlessButtonStyle).touchTarget(context).apply { text = value }

/**
 * La acción principal deshabilitada conserva su color, más tenue: sigue
 * pareciendo la principal mientras falta marcar la casilla.
 */
private fun MaterialButton.primaryEvenWhenDisabled(context: Context) = apply {
    val disabled = intArrayOf(-android.R.attr.state_enabled)
    val primary = ContextCompat.getColor(context, R.color.primary)
    val onPrimary = ContextCompat.getColor(context, R.color.on_primary)
    backgroundTintList = ColorStateList(arrayOf(disabled, intArrayOf()),
        intArrayOf(ColorUtils.setAlphaComponent(primary, 0x73), primary))
    setTextColor(ColorStateList(arrayOf(disabled, intArrayOf()),
        intArrayOf(ColorUtils.setAlphaComponent(onPrimary, 0xD9), onPrimary)))
}

/** Consentimiento nuevo en cada presentación, incluida una rotación o un rechazo del núcleo. */
class UnsafeExceptionDialog(
    private val context: Context,
    val item: UnsafeExceptionState,
    fromBlock: Boolean,
    onActivate: (Boolean) -> Unit,
    onCancel: () -> Unit,
) {
    private val title = EngineText.resolve(context, titleKey(item, fromBlock))
    /** Solo el texto se desplaza; la casilla y los botones quedan siempre a la vista. */
    private val body = column(context).apply { setPadding(context.dp(24), context.dp(12), context.dp(24), context.dp(12)) }
    private val footer = column(context).apply {
        id = R.id.unsafeFooter
        setPadding(context.dp(24), context.dp(8), context.dp(24), context.dp(16))
    }
    val acceptance = MaterialCheckBox(context).apply {
        id = R.id.unsafeAcceptance
        text = UnsafeExceptionTexts.text(context, "aceptacion_casilla")
        minHeight = context.dp(48)
        isSingleLine = false
        visibility = if (item.canActivate) View.VISIBLE else View.GONE
        layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
    }
    private val status = paragraph(context, "").apply {
        id = R.id.unsafeStatus
        accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE
        visibility = View.GONE
    }
    val activate = action(context, UnsafeExceptionTexts.text(context, "activar_continuar")).primaryEvenWhenDisabled(context).apply {
        id = R.id.unsafeActivate
        visibility = if (item.canActivate) View.VISIBLE else View.GONE
        isEnabled = false
        setOnClickListener { if (isEnabled && acceptance.isChecked) onActivate(true) }
    }
    private val cancel = secondary(context, UnsafeExceptionTexts.text(context, "cancelar"), outlined = true).apply {
        id = R.id.unsafeCancel
        setOnClickListener { onCancel() }
    }
    private val more = secondary(context, UnsafeExceptionTexts.text(context, "mas_informacion"), outlined = false).apply {
        id = R.id.unsafeMore
        setOnClickListener { showHelp(context, item) }
    }
    private var busy = false
    private val dialog: AlertDialog

    init {
        ViewCompat.setAccessibilityPaneTitle(body, title)
        body.addView(paragraph(context, title, heading = true))
        body.addView(paragraph(context, EngineText.resolve(context, item.key("ajuste")), heading = true))
        body.addView(paragraph(context, EngineText.resolve(context, item.key("riesgo"))))
        if (!item.activable || item.fixed || item.policyUnreadable) {
            body.addView(paragraph(context, EngineText.resolve(context, item.key("organizacion"))))
        } else if (item.acceptanceVersion.isBlank()) {
            body.addView(paragraph(context, UnsafeExceptionTexts.text(context, "version_no_disponible")))
        }
        if (item.canActivate) {
            // La versión aceptada viaja al núcleo con la activación; a la persona no le sirve para decidir.
            body.addView(paragraph(context, EngineText.resolve(context, item.key("aceptacion"))).apply {
                id = R.id.unsafeAcceptanceText
                labelFor = acceptance.id
                setTextIsSelectable(true)
            })
            body.addView(paragraph(context, UnsafeExceptionTexts.text(context, "desactivar_aqui")))
        }
        val scroll = ScrollView(context).apply {
            id = R.id.unsafeScroll
            // Con letra grande la barra y el borde difuminado indican que el texto sigue.
            isVerticalScrollBarEnabled = true
            isScrollbarFadingEnabled = false
            isVerticalFadingEdgeEnabled = true
            setFadingEdgeLength(context.dp(32))
            addView(body)
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f)
        }
        footer.addView(View(context).apply {
            setBackgroundColor(ContextCompat.getColor(context, R.color.outline_variant))
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, context.dp(1)).apply {
                bottomMargin = context.dp(8)
            }
        })
        // Sin activación posible la casilla no se añade: ni se ve ni la recorre el lector de pantalla.
        if (item.canActivate) footer.addView(acceptance)
        footer.addView(status)
        footer.addView(activate)
        // Las secundarias van aparte, con otro estilo y separadas de la principal.
        val large = context.resources.configuration.fontScale > 1.3f
        val secondaries = LinearLayout(context).apply {
            orientation = if (large) LinearLayout.VERTICAL else LinearLayout.HORIZONTAL
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                topMargin = context.dp(if (item.canActivate) 12 else 0)
            }
        }
        listOf(more, cancel).forEachIndexed { index, button ->
            button.layoutParams = if (large) LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
                else LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f).apply {
                    if (index > 0) marginStart = context.dp(8)
                }
            secondaries.addView(button)
        }
        footer.addView(secondaries)
        val root = column(context).apply {
            addView(scroll)
            addView(footer)
        }
        acceptance.setOnCheckedChangeListener { _, checked -> activate.isEnabled = checked && item.canActivate && !busy }
        dialog = MaterialAlertDialogBuilder(context).setView(root).create()
        // Título de la ventana para TalkBack: el mismo que se ve, no el nombre del ajuste.
        dialog.window?.setAccessibleTitle(title)
        dialog.setOnCancelListener { onCancel() }
    }

    fun show() {
        dialog.show()
        dialog.window?.let {
            it.setAccessibleTitle(title)
            it.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
            SafeWindowInsets.fullScreenDialog(it)
        }
    }

    fun render(saving: Boolean, error: UiText?) {
        busy = saving
        dialog.setCancelable(!saving)
        dialog.setCanceledOnTouchOutside(false)
        acceptance.isEnabled = !saving
        activate.isEnabled = !saving && acceptance.isChecked && item.canActivate
        cancel.isEnabled = !saving
        more.isEnabled = !saving
        status.text = if (saving) UnsafeExceptionTexts.text(context, "guardando") else error?.resolve(context).orEmpty()
        status.visibility = if (saving || error != null) View.VISIBLE else View.GONE
    }

    fun close() = dialog.dismiss()

    companion object {
        /** Con un bloqueo, el del caso; desde Preferencias nada se ha bloqueado todavía. */
        fun titleKey(item: UnsafeExceptionState, fromBlock: Boolean): String =
            if (fromBlock) item.key("titulo") else "android.inseguro.aceptar_titulo"

        fun showHelp(context: Context, item: UnsafeExceptionState) {
            val body = column(context).apply {
                setPadding(context.dp(24), context.dp(12), context.dp(24), context.dp(24))
            }
            ViewCompat.setAccessibilityPaneTitle(body, EngineText.resolve(context, item.key("ajuste")))
            body.addView(paragraph(context, EngineText.resolve(context, item.key("ajuste")), heading = true))
            body.addView(paragraph(context, EngineText.resolve(context, item.key("ayuda"))).apply {
                id = R.id.unsafeHelpText
                setTextIsSelectable(true)
            })
            val close = action(context, UnsafeExceptionTexts.text(context, "cerrar"))
            body.addView(close)
            val dialog = MaterialAlertDialogBuilder(context).setView(ScrollView(context).apply { addView(body) }).create()
            close.setOnClickListener { dialog.dismiss() }
            dialog.show()
            dialog.window?.let {
                it.setAccessibleTitle(EngineText.resolve(context, item.key("ajuste")))
                it.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
                SafeWindowInsets.fullScreenDialog(it)
            }
        }
    }
}

/** La sección vive dentro del ScrollView de Preferencias. No guarda un segundo estado de los ajustes. */
class UnsafeExceptionsPanel(
    private val container: LinearLayout,
    private val refresh: () -> Unit,
    private val activate: (UnsafeExceptionState) -> Unit,
    private val disable: (UnsafeExceptionState) -> Unit,
) {
    private val context = container.context
    private val status = paragraph(context, "").apply { accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE }
    private val rows = column(context)
    private val refreshButton = action(context, UnsafeExceptionTexts.text(context, "actualizar")).apply {
        id = R.id.unsafeRefresh
        setOnClickListener { refresh() }
    }
    private var items: List<UnsafeExceptionState>? = null
    private val controls = mutableListOf<Pair<MaterialButton, Boolean>>()
    init {
        container.addView(paragraph(context, UnsafeExceptionTexts.text(context, "seccion"), heading = true))
        container.addView(paragraph(context, UnsafeExceptionTexts.text(context, "introduccion")))
        container.addView(status)
        container.addView(refreshButton)
        container.addView(rows)
    }

    fun render(state: MainUiState) {
        status.text = when {
            !state.unsafeExceptionsAvailable -> UnsafeExceptionTexts.text(context, "no_disponible")
            state.unsafeSaving -> UnsafeExceptionTexts.text(context, "guardando")
            state.unsafeLoading -> UnsafeExceptionTexts.text(context, "cargando")
            else -> state.unsafeError?.resolve(context).orEmpty()
        }
        status.visibility = if (status.text.isEmpty()) View.GONE else View.VISIBLE
        refreshButton.isEnabled = state.unsafeExceptionsAvailable && !state.unsafeLoading && !state.unsafeSaving
        if (items != state.unsafeExceptions) {
            items = state.unsafeExceptions
            rows.removeAllViews()
            controls.clear()
            state.unsafeExceptions.forEach { item ->
                val name = EngineText.resolve(context, item.key("ajuste"))
                val row = column(context).apply { tag = "unsafe-case-${item.id}" }
                row.addView(paragraph(context, name, heading = true))
                row.addView(paragraph(context, EngineText.resolve(context, item.statusKey)))
                if (item.enabled && item.acceptedAt.isNotBlank()) row.addView(paragraph(context,
                    UnsafeExceptionTexts.text(context, "aceptada_fecha").replace("{fecha}", UiText.DateTime(item.acceptedAt, true).resolve(context))))
                if (item.fixed || item.policyUnreadable) row.addView(paragraph(context, EngineText.resolve(context, item.key("organizacion"))))
                val change = action(context, UnsafeExceptionTexts.text(context, when {
                    item.enabled -> "desactivar"
                    !item.activable -> "ver_bloqueo"
                    else -> "activar"
                })).apply {
                    tag = "unsafe-change-${item.id}"
                    contentDescription = "$text: $name"
                    setOnClickListener { if (item.enabled) disable(item) else activate(item) }
                }
                controls += change to (if (item.enabled) item.canDisable else true)
                row.addView(change)
                row.addView(action(context, UnsafeExceptionTexts.text(context, "mas_informacion")).apply {
                    tag = "unsafe-help-${item.id}"
                    contentDescription = "$text: $name"
                    setOnClickListener { UnsafeExceptionDialog.showHelp(context, item) }
                })
                rows.addView(row)
            }
        }
        controls.forEach { (button, changeable) ->
            button.isEnabled = changeable && !state.unsafeLoading && !state.unsafeSaving && state.canReplaceSelection
        }
    }
}
