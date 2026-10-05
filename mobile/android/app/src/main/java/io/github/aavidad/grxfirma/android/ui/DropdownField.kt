// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.os.Bundle
import android.os.Parcelable
import android.text.InputType
import android.util.AttributeSet
import android.widget.ArrayAdapter
import android.widget.Filter
import androidx.core.os.BundleCompat
import com.google.android.material.textfield.MaterialAutoCompleteTextView

/**
 * Desplegable de solo lectura dentro de un TextInputLayout
 * «ExposedDropdownMenu»: se ve como un campo, lleva su etiqueta y no corta las
 * opciones largas. Ofrece la misma interfaz mínima que usaba el Spinner.
 */
class DropdownField @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = androidx.appcompat.R.attr.autoCompleteTextViewStyle,
) : MaterialAutoCompleteTextView(context, attrs, defStyleAttr) {
    private var items: List<String> = emptyList()
    private var pendingRestore = -1

    var selectedItemPosition: Int = 0
        private set

    /** Solo se llama cuando la persona elige una opción, no al fijarla en código. */
    var onItemSelected: ((Int) -> Unit)? = null

    init {
        inputType = InputType.TYPE_NULL
        isCursorVisible = false
        keyListener = null
        val styled = context.obtainStyledAttributes(attrs, intArrayOf(android.R.attr.entries))
        val entries = styled.getTextArray(0)
        styled.recycle()
        if (entries != null) setItems(entries.map { it.toString() })
        setOnItemClickListener { _, _, position, _ ->
            selectedItemPosition = position
            onItemSelected?.invoke(position)
        }
    }

    fun setItems(values: List<String>) {
        items = values
        setAdapter(UnfilteredAdapter(context, values))
        val target = if (pendingRestore >= 0) pendingRestore else selectedItemPosition
        pendingRestore = -1
        select(target)
    }

    /** Fija la opción sin avisar al oyente, como hacía el código con el Spinner. */
    fun select(index: Int) {
        if (items.isEmpty()) {
            selectedItemPosition = 0
            return
        }
        selectedItemPosition = index.coerceIn(0, items.lastIndex)
        setText(items[selectedItemPosition], false)
    }

    override fun onSaveInstanceState(): Parcelable = Bundle().apply {
        putParcelable(SUPER, super.onSaveInstanceState())
        putInt(POSITION, selectedItemPosition)
    }

    override fun onRestoreInstanceState(state: Parcelable?) {
        if (state !is Bundle) return super.onRestoreInstanceState(state)
        super.onRestoreInstanceState(BundleCompat.getParcelable(state, SUPER, Parcelable::class.java))
        val position = state.getInt(POSITION, 0)
        if (items.isEmpty()) pendingRestore = position else select(position)
    }

    /** El filtro de AutoComplete ocultaría las demás opciones tras elegir una. */
    private class UnfilteredAdapter(context: Context, private val values: List<String>) :
        ArrayAdapter<String>(context, android.R.layout.simple_list_item_1, values) {
        override fun getFilter(): Filter = object : Filter() {
            override fun performFiltering(constraint: CharSequence?) =
                FilterResults().apply { this.values = this@UnfilteredAdapter.values; count = this@UnfilteredAdapter.values.size }
            override fun publishResults(constraint: CharSequence?, results: FilterResults?) = notifyDataSetChanged()
        }
    }

    private companion object {
        const val SUPER = "super"
        const val POSITION = "position"
    }
}
