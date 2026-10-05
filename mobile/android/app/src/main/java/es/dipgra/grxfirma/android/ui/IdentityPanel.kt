// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import android.content.Context
import android.view.Gravity
import android.view.View
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.view.isNotEmpty
import com.google.android.material.button.MaterialButton
import com.google.android.material.radiobutton.MaterialRadioButton
import es.dipgra.grxfirma.android.R

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
        val rows = shown.map { identity ->
            Row(identity.id, label(context, state, identity), identity.id == state.certificate?.id, identity.certificate.subject)
        }
        val key = rows to state.canChangeIdentity
        // Solo se reconstruye si algo cambia, para no mover el foco de TalkBack.
        if (list.tag == key) return
        list.tag = key
        list.removeAllViews()
        rows.forEach { row -> list.addView(rowView(context, row, state.canChangeIdentity, onSelect, onClose)) }
    }

    private data class Row(val id: String, val label: String, val selected: Boolean, val subject: String)

    private fun label(context: Context, state: MainUiState, identity: SessionIdentity): String {
        val detail = state.certificateDetails.firstOrNull { it.id == identity.id }
        val lines = buildList {
            add(identity.certificate.subject)
            if (detail != null) {
                add(context.getString(CertificateText.kindLabel(detail.kind)))
                if (detail.nif.isNotBlank()) add(context.getString(R.string.cert_nif, detail.nif))
                add(CertificateText.expiry(detail).resolve(context))
            }
            add(context.getString(if (identity.external) R.string.cert_origin_dnie else R.string.cert_origin_file))
        }
        return lines.joinToString("\n")
    }

    private fun rowView(
        context: Context,
        row: Row,
        enabled: Boolean,
        onSelect: (String) -> Unit,
        onClose: (String) -> Unit,
    ): View = LinearLayout(context).apply {
        orientation = LinearLayout.HORIZONTAL
        gravity = Gravity.CENTER_VERTICAL
        layoutParams = LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT)
        val radio = MaterialRadioButton(context).apply {
            text = row.label
            isChecked = row.selected
            isEnabled = enabled
            minHeight = context.resources.getDimensionPixelSize(R.dimen.touch_target)
            setOnClickListener { if (!row.selected) onSelect(row.id) else isChecked = true }
        }
        addView(radio, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
        val close = MaterialButton(context, null, android.R.attr.borderlessButtonStyle).apply {
            setText(R.string.identity_close)
            contentDescription = context.getString(R.string.identity_close_description, row.subject)
            isEnabled = enabled
            minHeight = context.resources.getDimensionPixelSize(R.dimen.touch_target)
            setOnClickListener { onClose(row.id) }
        }
        addView(close, LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT))
    }
}
