// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.annotation.SuppressLint
import android.content.Context
import android.content.res.ColorStateList
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Build
import android.view.View
import android.view.ViewGroup
import android.view.Window
import android.view.accessibility.AccessibilityNodeInfo
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

/**
 * Al abrir una ventana, TalkBack se queda con el primer control enfocable (la
 * casilla o «Cerrar») y se salta el título. El título pasa a ser enfocable y
 * recibe el foco de entrada y el de accesibilidad en cuanto se pinta.
 *
 * Se fuerza el foco de accesibilidad solo aquí, una vez al abrir el aviso o su
 * ayuda: quien lo use tiene que oír el título y el ajuste antes que la casilla
 * con la que acepta el riesgo (recorrido Android 3, defecto 1).
 */
@SuppressLint("AccessibilityFocus")
internal fun View.takeInitialFocus() {
    isFocusable = true
    isFocusableInTouchMode = true
    val focus = Runnable {
        requestFocus()
        performAccessibilityAction(AccessibilityNodeInfo.ACTION_ACCESSIBILITY_FOCUS, null)
    }
    post(focus)
    // TalkBack coloca su foco al recibir el cambio de ventana; se repite después.
    postDelayed(focus, 400)
}

private fun Context.dp(value: Int) = (value * resources.displayMetrics.density + 0.5f).toInt()
private fun column(context: Context) = LinearLayout(context).apply {
    orientation = LinearLayout.VERTICAL
}
/**
 * Cada párrafo y cada encabezado es una parada propia de TalkBack. Sin esto,
 * TalkBack junta todo el texto de una zona desplazable y lo lee de un tirón.
 */
private fun TextView.readAlone() = apply { ViewCompat.setScreenReaderFocusable(this, true) }
private fun paragraph(
    context: Context,
    value: CharSequence,
    heading: Boolean = false,
    headingStyle: Int = com.google.android.material.R.style.TextAppearance_Material3_TitleLarge,
) = TextView(context).apply {
    text = value
    setTextAppearance(if (heading) headingStyle else com.google.android.material.R.style.TextAppearance_Material3_BodyLarge)
    setPadding(0, context.dp(8), 0, context.dp(8))
    ViewCompat.setAccessibilityHeading(this, heading)
    readAlone()
}
/** Con letra grande dos botones no caben uno al lado del otro. */
private fun Context.largeText() = resources.configuration.fontScale > 1.3f
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
    val activate = action(context, EngineText.resolve(context, activateKey(fromBlock))).primaryEvenWhenDisabled(context).apply {
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
        setOnClickListener { showHelp(context, item) { titleView.takeInitialFocus() } }
    }
    private var busy = false
    private val dialog: AlertDialog
    private val titleView = paragraph(context, title, heading = true).apply { id = R.id.unsafeTitle }

    init {
        ViewCompat.setAccessibilityPaneTitle(body, title)
        body.addView(titleView)
        body.addView(paragraph(context, EngineText.resolve(context, item.key("ajuste")), heading = true))
        // Ningún párrafo es seleccionable: un texto seleccionable recibe el foco
        // al abrir la ventana y TalkBack empezaría por él, no por el título.
        bodyKeys(item).forEach { key ->
            body.addView(paragraph(context, EngineText.resolve(context, key)).apply {
                // La versión aceptada viaja al núcleo con la activación; a la persona no le sirve para decidir.
                if (key == item.key("aceptacion")) {
                    id = R.id.unsafeAcceptanceText
                    labelFor = acceptance.id
                }
            })
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
        val large = context.largeText()
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
            // La ventana se ajusta al contenido: sin hueco vacío bajo los botones.
            // Con letra grande crece hasta la pantalla y solo se desplaza el texto.
            it.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
            SafeWindowInsets.fullScreenDialog(it)
        }
        // TalkBack empieza por el título, no por la casilla.
        titleView.takeInitialFocus()
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

        /** Desde Preferencias no hay ninguna operación detenida que continuar. */
        fun activateKey(fromBlock: Boolean): String =
            if (fromBlock) "android.inseguro.activar_continuar" else "android.inseguro.activar"

        /**
         * Cada cosa se dice una vez. El texto de aceptación ya nombra el riesgo
         * y cómo desactivarlo, así que con él no se añade el riesgo. Sin
         * aceptación posible se explica el riesgo y por qué no se puede activar.
         */
        fun bodyKeys(item: UnsafeExceptionState): List<String> = when {
            item.canActivate -> listOf(item.key("aceptacion"))
            !item.activable || item.fixed || item.policyUnreadable -> listOf(item.key("riesgo"), item.key("organizacion"))
            else -> listOf(item.key("riesgo"), "android.inseguro.version_no_disponible")
        }

        /**
         * Apartados de la ayuda: separados por una línea en blanco y, cada uno,
         * «Encabezado: texto». Sin dos puntos al principio, el apartado va sin encabezado.
         */
        fun helpSections(text: String): List<Pair<String, String>> =
            text.split("\n\n").map { it.trim() }.filter { it.isNotEmpty() }.map { section ->
                val colon = section.indexOfFirst { it == ':' || it == '：' }
                if (colon <= 0 || colon > 60) "" to section
                else section.substring(0, colon).trim() to section.substring(colon + 1).trim()
                    .replaceFirstChar { it.titlecase() }
            }

        fun showHelp(context: Context, item: UnsafeExceptionState, onClosed: () -> Unit = {}) {
            val setting = EngineText.resolve(context, item.key("ajuste"))
            val body = column(context).apply {
                setPadding(context.dp(24), context.dp(12), context.dp(24), context.dp(12))
            }
            ViewCompat.setAccessibilityPaneTitle(body, setting)
            val helpTitle = paragraph(context, setting, heading = true).apply { id = R.id.unsafeHelpTitle }
            body.addView(helpTitle)
            // Un encabezado por apartado: TalkBack puede saltar de uno a otro.
            helpSections(EngineText.resolve(context, item.key("ayuda"))).forEachIndexed { index, (heading, text) ->
                if (heading.isNotEmpty()) body.addView(TextView(context).apply {
                    this.text = heading
                    setTextAppearance(com.google.android.material.R.style.TextAppearance_Material3_TitleSmall)
                    setPadding(0, context.dp(12), 0, 0)
                    ViewCompat.setAccessibilityHeading(this, true)
                    readAlone()
                })
                body.addView(paragraph(context, text).apply {
                    if (index == 0) id = R.id.unsafeHelpText
                    setPadding(0, context.dp(4), 0, context.dp(4))
                })
            }
            // El texto se desplaza con la barra siempre visible; «Cerrar» queda fijo abajo.
            val scroll = ScrollView(context).apply {
                id = R.id.unsafeHelpScroll
                isVerticalScrollBarEnabled = true
                isScrollbarFadingEnabled = false
                isVerticalFadingEdgeEnabled = true
                setFadingEdgeLength(context.dp(32))
                addView(body)
                layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f)
            }
            val close = action(context, UnsafeExceptionTexts.text(context, "cerrar")).apply { id = R.id.unsafeHelpClose }
            val footer = column(context).apply {
                setPadding(context.dp(24), context.dp(8), context.dp(24), context.dp(16))
                addView(View(context).apply {
                    setBackgroundColor(ContextCompat.getColor(context, R.color.outline_variant))
                    layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, context.dp(1)).apply {
                        bottomMargin = context.dp(8)
                    }
                })
                addView(close)
            }
            val root = column(context).apply {
                addView(scroll)
                addView(footer)
            }
            val dialog = MaterialAlertDialogBuilder(context).setView(root).create()
            close.setOnClickListener { dialog.dismiss() }
            dialog.setOnDismissListener { onClosed() }
            dialog.show()
            dialog.window?.let {
                it.setAccessibleTitle(setting)
                it.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
                SafeWindowInsets.fullScreenDialog(it)
            }
            helpTitle.takeInitialFocus()
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
    // Casi nadie lo necesita: solo texto y al final de la lista, por debajo de las acciones de cada caso.
    private val refreshButton = secondary(context, UnsafeExceptionTexts.text(context, "actualizar"), outlined = false).apply {
        id = R.id.unsafeRefresh
        layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT)
        setOnClickListener { refresh() }
    }
    private var items: List<UnsafeExceptionState>? = null
    private val controls = mutableMapOf<String, Pair<MaterialButton, Boolean>>()
    /** Caso cuyo botón se ha pulsado: al cambiar su estado, recibe el foco y se anuncia. */
    private var pendingFocus: String? = null
    /** «Ajuste: estado nuevo», en la región viva de la sección (TalkBack lo lee sin mover el foco). */
    private var announcement = ""
    init {
        container.addView(paragraph(context, UnsafeExceptionTexts.text(context, "seccion"), heading = true).apply {
            id = R.id.unsafeSectionTitle
        })
        container.addView(paragraph(context, UnsafeExceptionTexts.text(context, "introduccion")))
        container.addView(status)
        container.addView(rows)
        container.addView(refreshButton)
    }

    fun render(state: MainUiState) {
        renderRows(state)
        status.text = when {
            !state.unsafeExceptionsAvailable -> UnsafeExceptionTexts.text(context, "no_disponible")
            state.unsafeSaving -> UnsafeExceptionTexts.text(context, "guardando")
            state.unsafeLoading -> UnsafeExceptionTexts.text(context, "cargando")
            else -> state.unsafeError?.resolve(context) ?: announcement
        }
        status.visibility = if (status.text.isEmpty()) View.GONE else View.VISIBLE
        controls.values.forEach { (button, changeable) ->
            button.isEnabled = changeable && !state.unsafeLoading && !state.unsafeSaving && state.canReplaceSelection
        }
    }

    private fun renderRows(state: MainUiState) {
        refreshButton.isEnabled = state.unsafeExceptionsAvailable && !state.unsafeLoading && !state.unsafeSaving
        val previous = items
        if (previous != state.unsafeExceptions) {
            items = state.unsafeExceptions
            val changed = changedIds(previous, state.unsafeExceptions)
            if (changed == null) {
                pendingFocus = null
                announcement = ""
                rows.removeAllViews()
                controls.clear()
                state.unsafeExceptions.forEach { rows.addView(row(it)) }
            } else {
                // Solo se rehace la fila que cambia: las demás, y el foco, siguen donde estaban.
                state.unsafeExceptions.filter { it.id in changed }.forEach { item ->
                    val old = rows.findViewWithTag<View>("unsafe-case-${item.id}")
                    val index = rows.indexOfChild(old)
                    rows.removeViewAt(index)
                    val fresh = row(item)
                    rows.addView(fresh, index)
                    if (item.id == pendingFocus) {
                        pendingFocus = null
                        // Con teclado el foco vuelve al botón del caso; TalkBack oye el estado nuevo.
                        fresh.findViewWithTag<View>("unsafe-change-${item.id}").requestFocus()
                        val name = EngineText.resolve(context, item.key("ajuste"))
                        val now = fresh.findViewWithTag<TextView>("unsafe-status-${item.id}").text
                        announcement = "$name: $now"
                    }
                }
            }
        }
    }

    private fun row(item: UnsafeExceptionState): View {
        val name = EngineText.resolve(context, item.key("ajuste"))
        val row = column(context).apply {
            tag = "unsafe-case-${item.id}"
            setPadding(context.dp(12), context.dp(8), context.dp(12), context.dp(12))
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                topMargin = context.dp(8)
            }
            // Un caso activado se ve de un vistazo, no solo por la palabra «Activada».
            if (item.enabled) background = GradientDrawable().apply {
                cornerRadius = context.dp(12).toFloat()
                setColor(ContextCompat.getColor(context, R.color.status_warning_container))
            }
        }
        // Un escalón por debajo del título «Seguridad»: la jerarquía se ve, no solo la oye TalkBack.
        row.addView(paragraph(context, name, heading = true,
            headingStyle = com.google.android.material.R.style.TextAppearance_Material3_TitleMedium).apply {
            tag = "unsafe-name-${item.id}"
        })
        // Una sola línea de estado: «Activado el …» ya dice que está activado.
        val key = statusKey(item)
        val statusText = EngineText.resolve(context, key).let {
            if (key == DATE_KEY) it.replace("{date}", UiText.DateTime(item.acceptedAt, true).resolve(context)) else it
        }
        row.addView(paragraph(context, statusText).apply {
            tag = "unsafe-status-${item.id}"
            if (item.enabled) setTypeface(typeface, Typeface.BOLD)
        })
        if (item.fixed || item.policyUnreadable) row.addView(paragraph(context, EngineText.resolve(context, item.key("organizacion"))))
        val change = action(context, UnsafeExceptionTexts.text(context, when {
            item.enabled -> "desactivar"
            !item.activable -> "ver_bloqueo"
            else -> "activar"
        })).apply {
            tag = "unsafe-change-${item.id}"
            contentDescription = "$text: $name"
            setOnClickListener {
                pendingFocus = item.id
                if (item.enabled) disable(item) else activate(item)
            }
        }
        controls[item.id] = change to (if (item.enabled) item.canDisable else true)
        // Una sola acción rellena por caso; «Más información» va como texto.
        val help = secondary(context, UnsafeExceptionTexts.text(context, "mas_informacion"), outlined = false).apply {
            tag = "unsafe-help-${item.id}"
            contentDescription = "$text: $name"
            setOnClickListener { UnsafeExceptionDialog.showHelp(context, item) }
        }
        val large = context.largeText()
        val actions = LinearLayout(context).apply {
            orientation = if (large) LinearLayout.VERTICAL else LinearLayout.HORIZONTAL
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
        }
        listOf(change, help).forEachIndexed { index, button ->
            button.layoutParams = if (large) LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
                else LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f).apply {
                    if (index > 0) marginStart = context.dp(8)
                }
            actions.addView(button)
        }
        row.addView(actions)
        return row
    }

    companion object {
        private const val DATE_KEY = "android.inseguro.aceptada_fecha"

        /** Activado por la persona y con fecha: una línea, «Activado el {date}». */
        fun statusKey(item: UnsafeExceptionState): String =
            if (item.statusKey == "android.inseguro.activada" && item.acceptedAt.isNotBlank()) DATE_KEY else item.statusKey

        /** Casos que cambian entre dos listas con los mismos casos en el mismo orden; null si hay que rehacerlas. */
        fun changedIds(old: List<UnsafeExceptionState>?, new: List<UnsafeExceptionState>): Set<String>? {
            if (old == null || old.map { it.id } != new.map { it.id }) return null
            return old.zip(new).filter { (a, b) -> a != b }.map { it.second.id }.toSet()
        }
    }
}
