// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.view.Gravity
import android.view.View
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.core.view.isNotEmpty
import com.google.android.material.color.MaterialColors
import com.google.android.material.button.MaterialButton
import com.google.android.material.radiobutton.MaterialRadioButton
import io.github.aavidad.grxfirma.android.R

/**
 * Lista de certificados abiertos: un botón de opción por certificado para
 * elegir con cuál firmar (TalkBack anuncia cuál está marcado) y un botón
 * «Cerrar» en cada fila. Solo se muestra con más de un certificado.
 */
object IdentityPanel {
    fun render(
        context: Context,
        state: MainUiState,
        group: View,
        title: TextView,
        list: LinearLayout,
        onSelect: (String) -> Unit,
        onClose: (String) -> Unit,
    ) {
        if (!state.showsIdentityList) {
            group.visibility = View.GONE
            if (list.isNotEmpty()) list.removeAllViews()
            list.tag = null
            return
        }
        group.visibility = View.VISIBLE
        title.text = context.resources.getQuantityString(R.plurals.identities_open, state.identities.size, state.identities.size)
        val shown = state.filteredIdentities
        val labels = shown.associate { identity -> identity.id to label(context, state, identity) }
        val rows = shown.map { identity ->
            Row(identity.id, labels.getValue(identity.id).toString(), identity.id == state.certificate?.id, identity.certificate.subject)
        }
        val key = rows to state.canChangeIdentity
        // Solo se reconstruye si algo cambia, para no mover el foco de TalkBack.
        if (list.tag == key) return
        list.tag = key
        list.removeAllViews()
        rows.forEachIndexed { index, row ->
            // Un divisor entre filas: cada certificado se distingue del siguiente.
            if (index > 0) list.addView(divider(context))
            list.addView(rowView(context, row, labels.getValue(row.id), state.canChangeIdentity, onSelect, onClose))
        }
    }

    private data class Row(val id: String, val label: String, val selected: Boolean, val subject: String)

    /** El elegido lleva su detalle completo junto a su fila; los demás, un resumen. */
    private fun label(context: Context, state: MainUiState, identity: SessionIdentity): CharSequence {
        val detail = state.certificateDetails.firstOrNull { it.id == identity.id }
        if (detail != null && identity.id == state.certificate?.id) {
            return CertificateText.styled(context, detail, heading = identity.certificate.subject)
        }
        val text = android.text.SpannableStringBuilder(identity.certificate.subject)
        if (detail != null) {
            text.append("\n").append(context.getString(CertificateText.kindLabel(detail.kind)))
            if (detail.nif.isNotBlank()) text.append("\n").append(context.getString(R.string.cert_nif, detail.nif))
            text.append("\n")
            val start = text.length
            text.append(CertificateText.expiry(detail).resolve(context))
            // El mismo aviso de caducidad que en el certificado elegido.
            if (CertificateText.warns(detail)) {
                text.setSpan(android.text.style.ForegroundColorSpan(ContextCompat.getColor(context, R.color.status_warning)),
                    start, text.length, android.text.Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                text.setSpan(android.text.style.StyleSpan(android.graphics.Typeface.BOLD), start, text.length,
                    android.text.Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            }
        }
        text.append("\n").append(context.getString(if (identity.external) R.string.cert_origin_dnie else R.string.cert_origin_file))
        return text
    }

    private fun divider(context: Context): View = View(context).apply {
        setBackgroundColor(MaterialColors.getColor(context, com.google.android.material.R.attr.colorOutlineVariant, 0))
        importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO
        layoutParams = LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT,
            context.resources.displayMetrics.density.toInt().coerceAtLeast(1))
    }

    private fun rowView(
        context: Context,
        row: Row,
        label: CharSequence,
        enabled: Boolean,
        onSelect: (String) -> Unit,
        onClose: (String) -> Unit,
    ): View = LinearLayout(context).apply {
        orientation = LinearLayout.HORIZONTAL
        gravity = Gravity.CENTER_VERTICAL
        layoutParams = LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT)
        val gap = (8 * context.resources.displayMetrics.density).toInt()
        setPadding(0, gap, 0, gap)
        val radio = MaterialRadioButton(context).apply {
            text = label
            isChecked = row.selected
            isEnabled = enabled
            minHeight = context.resources.getDimensionPixelSize(R.dimen.touch_target)
            setOnClickListener { if (!row.selected) onSelect(row.id) else isChecked = true }
        }
        addView(radio, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
        // Botón de texto del tema Material 3: «Cerrar» sin mayúsculas forzadas, como el resto.
        val close = MaterialButton(context, null, androidx.appcompat.R.attr.borderlessButtonStyle).apply {
            isAllCaps = false
            setText(R.string.identity_close)
            contentDescription = context.getString(R.string.identity_close_description, row.subject)
            isEnabled = enabled
            minHeight = context.resources.getDimensionPixelSize(R.dimen.touch_target)
            setOnClickListener { onClose(row.id) }
        }
        addView(close, LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT))
    }
}
