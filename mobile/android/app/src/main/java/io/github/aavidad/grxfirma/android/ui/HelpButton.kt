// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.graphics.Typeface
import android.text.SpannableString
import android.text.Spanned
import android.text.style.StyleSpan
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import androidx.annotation.StringRes
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.widget.TooltipCompat
import androidx.core.view.ViewCompat
import com.google.android.material.button.MaterialButton
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import io.github.aavidad.grxfirma.android.R

/**
 * Botón «?» de ayuda contextual, igual en todas las pantallas: icono de ayuda
 * en 48 dp, nombre accesible «Ayuda sobre …» (también como tooltip) y, al
 * pulsarlo, un diálogo con el texto. El estilo vive en
 * `Widget.GrxFirma.HelpButton`; aquí solo se le da nombre y contenido.
 *
 * Cada apartado del diálogo es una frase (con el nombre de la opción en
 * negrita si lo tiene) y, si tiene ampliación, un botón «+» al final que la
 * despliega y pasa a «−»; el diálogo se desplaza si el texto no cabe.
 */
object HelpButton {
    /** Nombre que lee TalkBack y muestra la pulsación larga. */
    fun accessibleName(context: Context, controlName: CharSequence): String =
        context.getString(R.string.ayuda_boton_nombre, controlName)

    /** Prepara un «?» del XML: [controlName] es la etiqueta visible del control al que acompaña. */
    fun bind(button: View, controlName: CharSequence, message: () -> CharSequence) {
        val name = accessibleName(button.context, controlName)
        button.contentDescription = name
        TooltipCompat.setTooltipText(button, name)
        button.setOnClickListener { show(button.context, controlName, message()) }
    }

    /** Un texto suelto; si tiene ampliación («…_mas»), lleva su «+». */
    fun bind(button: View, @StringRes controlName: Int, @StringRes text: Int) =
        bindSections(button, controlName) { listOf(ContextHelp.single(text)) }

    /** Ayuda de varios apartados (p. ej. una por opción de un desplegable), con el nombre de cada opción en negrita. */
    fun bindSections(button: View, @StringRes controlName: Int, sections: () -> List<ContextHelp.Section>) {
        val title = button.context.getString(controlName)
        val name = accessibleName(button.context, title)
        button.contentDescription = name
        TooltipCompat.setTooltipText(button, name)
        button.setOnClickListener { showSections(button.context, title, sections()) }
    }

    fun show(context: Context, title: CharSequence, message: CharSequence): AlertDialog =
        MaterialAlertDialogBuilder(context)
            .setTitle(title)
            .setMessage(message)
            .setPositiveButton(R.string.help_close, null)
            .show()

    fun showSections(context: Context, title: CharSequence, sections: List<ContextHelp.Section>): AlertDialog =
        MaterialAlertDialogBuilder(context)
            .setTitle(title)
            .setView(sectionsView(context, title, sections))
            .setPositiveButton(R.string.help_close, null)
            .show()

    /** «?» creado en código, para las vistas que no salen de un XML (editor del sello). */
    fun create(context: Context, @StringRes controlName: Int, @StringRes text: Int): MaterialButton =
        MaterialButton(context, null, R.attr.grxHelpButtonStyle).apply {
            val size = (48 * resources.displayMetrics.density).toInt()
            layoutParams = LinearLayout.LayoutParams(size, size).apply { marginStart = size / 12 }
            bind(this, controlName, text)
        }

    /**
     * Pone el «?» a la derecha de [control] en una fila; el control ocupa el
     * resto del ancho. Devuelve la fila: es la que se muestra u oculta.
     */
    fun row(context: Context, control: View, @StringRes controlName: Int, @StringRes text: Int): LinearLayout =
        LinearLayout(context).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            addView(control, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
            addView(create(context, controlName, text))
        }

    private fun sectionsView(context: Context, topic: CharSequence, sections: List<ContextHelp.Section>): View {
        val density = context.resources.displayMetrics.density
        val column = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            val side = (24 * density).toInt()
            setPadding(side, (8 * density).toInt(), side, 0)
        }
        sections.forEach { column.addView(sectionView(context, topic, it)) }
        return ScrollView(context).apply { addView(column) }
    }

    private fun sectionView(context: Context, topic: CharSequence, section: ContextHelp.Section): View {
        val density = context.resources.displayMetrics.density
        val name = section.title?.let(context::getString)
        val text = context.getString(section.text)
        val sentence = TextView(context).apply {
            setTextAppearance(com.google.android.material.R.style.TextAppearance_Material3_BodyMedium)
            setTextIsSelectable(true)
            this.text = if (name == null) text else {
                // Plantilla sin rellenar («%1$s: %2$s»): el nombre va en negrita.
                val (composed, bold) = ContextHelp.compose(context.getString(R.string.ayuda_opcion), name, text)
                SpannableString(composed).apply {
                    if (!bold.isEmpty()) setSpan(StyleSpan(Typeface.BOLD), bold.first, bold.last + 1, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                }
            }
        }
        val wrapper = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(0, 0, 0, (12 * density).toInt())
        }
        val line = LinearLayout(context).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.TOP
            addView(sentence, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        }
        wrapper.addView(line)
        val moreRes = section.more ?: return wrapper
        val detail = TextView(context).apply {
            setTextAppearance(com.google.android.material.R.style.TextAppearance_Material3_BodyMedium)
            setTextIsSelectable(true)
            this.text = context.getString(moreRes)
            visibility = View.GONE
            setPadding(0, (4 * density).toInt(), 0, 0)
        }
        val size = (48 * density).toInt()
        val more = MaterialButton(context, null, com.google.android.material.R.attr.materialButtonOutlinedStyle).apply {
            tag = MORE_TAG
            minWidth = size
            minimumWidth = size
            minHeight = size
            minimumHeight = size
            insetTop = 0
            insetBottom = 0
            setPadding(0, 0, 0, 0)
            iconPadding = 0
            iconGravity = MaterialButton.ICON_GRAVITY_TEXT_START
            contentDescription = context.getString(R.string.ayuda_mas_nombre, name ?: topic)
            TooltipCompat.setTooltipText(this, contentDescription)
        }
        fun render(expanded: Boolean) {
            more.setIconResource(if (expanded) R.drawable.ic_help_more_remove else R.drawable.ic_help_more_add)
            ViewCompat.setStateDescription(more,
                context.getString(if (expanded) R.string.ayuda_mas_desplegado else R.string.ayuda_mas_plegado))
            detail.visibility = if (expanded) View.VISIBLE else View.GONE
        }
        render(false)
        more.setOnClickListener { render(detail.visibility != View.VISIBLE) }
        line.addView(more, LinearLayout.LayoutParams(size, size).apply { marginStart = (8 * density).toInt() })
        wrapper.addView(detail)
        return wrapper
    }

    /** Marca de los botones «+» del diálogo, para encontrarlos en las pruebas. */
    const val MORE_TAG = "grx-help-more"
}

/** Qué texto de ayuda corresponde a cada opción. Sin vistas, para probarlo en la JVM. */
object ContextHelp {
    /** Un apartado: nombre de la opción (o ninguno), frase breve y ampliación opcional. */
    data class Section(@StringRes val title: Int?, @StringRes val text: Int, @StringRes val more: Int? = null)

    /** Ampliaciones: solo las opciones que necesitan más explicación. */
    private val MORE = mapOf(
        R.string.ayuda_operacion_cofirma to R.string.ayuda_operacion_cofirma_mas,
        R.string.ayuda_operacion_contrafirma to R.string.ayuda_operacion_contrafirma_mas,
        R.string.ayuda_formato_pades to R.string.ayuda_formato_pades_mas,
        R.string.ayuda_formato_cades to R.string.ayuda_formato_cades_mas,
        R.string.ayuda_formato_xades to R.string.ayuda_formato_xades_mas,
        R.string.ayuda_formato_facturae to R.string.ayuda_formato_facturae_mas,
        R.string.ayuda_formato_asic to R.string.ayuda_formato_asic_mas,
        R.string.ayuda_sellado_tiempo to R.string.ayuda_sellado_tiempo_mas,
    )

    @StringRes
    fun more(@StringRes text: Int): Int? = MORE[text]

    fun single(@StringRes text: Int): Section = Section(null, text, more(text))

    private fun option(@StringRes title: Int, @StringRes text: Int): Section = Section(title, text, more(text))

    /** Texto general y las tres operaciones, en el orden del desplegable, sin depender de la elegida. */
    val OPERATIONS = listOf(
        single(R.string.ayuda_operacion),
        option(R.string.action_sign, R.string.ayuda_operacion_firma),
        option(R.string.action_cosign, R.string.ayuda_operacion_cofirma),
        option(R.string.action_countersign, R.string.ayuda_operacion_contrafirma),
    )

    private val FORMAT_TEXT = mapOf(
        "auto" to R.string.ayuda_formato_automatico,
        "pades" to R.string.ayuda_formato_pades,
        "cades" to R.string.ayuda_formato_cades,
        "xades" to R.string.ayuda_formato_xades,
        "xmldsig" to R.string.ayuda_formato_xmldsig,
        "odf" to R.string.ayuda_formato_odf,
        "ooxml" to R.string.ayuda_formato_ooxml,
        "facturae" to R.string.ayuda_formato_facturae,
        "asic-xades" to R.string.ayuda_formato_asic,
        "verifactu" to R.string.ayuda_formato_verifactu,
    )

    @StringRes
    fun formatText(format: String): Int? = FORMAT_TEXT[format]

    /** Texto general del formato y todas las opciones del desplegable [menu], cada una con su nombre. */
    fun formats(menu: List<String>): List<Section> = buildList {
        add(single(R.string.ayuda_formato))
        menu.forEach { format -> formatText(format)?.let { add(option(FormatPolicy.label(format), it)) } }
    }

    /**
     * Rellena la plantilla «%1$s: %2$s» (o la de cada idioma) con el nombre y
     * la frase, y dice qué caracteres ocupa el nombre para ponerlo en negrita.
     */
    fun compose(template: String, name: String, text: String): Pair<String, IntRange> {
        val nameAt = template.indexOf("%1\$s")
        val result = StringBuilder()
        var bold = IntRange.EMPTY
        var index = 0
        while (index < template.length) {
            when {
                template.startsWith("%1\$s", index) -> {
                    bold = result.length until result.length + name.length
                    result.append(name)
                    index += 4
                }
                template.startsWith("%2\$s", index) -> {
                    result.append(text)
                    index += 4
                }
                else -> result.append(template[index++])
            }
        }
        if (nameAt < 0) bold = IntRange.EMPTY
        return result.toString() to bold
    }
}
