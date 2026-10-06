// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.graphics.Typeface
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.style.StyleSpan
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import androidx.annotation.StringRes
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.widget.TooltipCompat
import com.google.android.material.button.MaterialButton
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import io.github.aavidad.grxfirma.android.R

/**
 * Botón «?» de ayuda contextual, igual en todas las pantallas: icono de ayuda
 * en 48 dp, nombre accesible «Ayuda sobre …» (también como tooltip) y, al
 * pulsarlo, un diálogo con el texto. El estilo vive en
 * `Widget.GrxFirma.HelpButton`; aquí solo se le da nombre y contenido.
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

    fun bind(button: View, @StringRes controlName: Int, @StringRes text: Int) =
        bind(button, button.context.getString(controlName)) { button.context.getString(text) }

    /** Ayuda de varios apartados (p. ej. una por opción de un desplegable), con el título de cada uno en negrita. */
    fun bindSections(button: View, @StringRes controlName: Int, sections: () -> List<ContextHelp.Section>) =
        bind(button, button.context.getString(controlName)) { render(button.context, sections()) }

    fun show(context: Context, title: CharSequence, message: CharSequence): AlertDialog =
        MaterialAlertDialogBuilder(context)
            .setTitle(title)
            .setMessage(message)
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
            gravity = android.view.Gravity.CENTER_VERTICAL
            addView(control, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
            addView(create(context, controlName, text))
        }

    private fun render(context: Context, sections: List<ContextHelp.Section>): CharSequence {
        val text = SpannableStringBuilder()
        sections.forEach { section ->
            if (text.isNotEmpty()) text.append("\n\n")
            section.title?.let {
                val start = text.length
                text.append(context.getString(it))
                text.setSpan(StyleSpan(Typeface.BOLD), start, text.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                text.append("\n")
            }
            text.append(context.getString(section.text))
        }
        return text
    }
}

/** Qué texto de ayuda corresponde a cada opción. Sin vistas, para probarlo en la JVM. */
object ContextHelp {
    data class Section(@StringRes val title: Int?, @StringRes val text: Int)

    /** Las tres operaciones, en el orden del desplegable, con su etiqueta como título. */
    val OPERATIONS = listOf(
        Section(R.string.action_sign, R.string.ayuda_operacion_firma),
        Section(R.string.action_cosign, R.string.ayuda_operacion_cofirma),
        Section(R.string.action_countersign, R.string.ayuda_operacion_contrafirma),
    )

    private val FORMAT_TEXT = mapOf(
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

    /** Texto general del formato y, si hay uno elegido, el de ese formato con su nombre. */
    fun format(format: String): List<Section> = buildList {
        add(Section(null, R.string.ayuda_formato))
        formatText(format)?.let { add(Section(FormatPolicy.label(format), it)) }
    }
}
