// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import com.google.android.material.textfield.TextInputLayout

/**
 * Errores de formulario (WCAG 3.3.1): tras validar, el foco va al primer campo
 * con error, para que el teclado y TalkBack lleguen a él sin buscarlo.
 */
object FieldErrors {
    /** Devuelve true si algún campo visible tenía error y ha recibido el foco. */
    fun focusFirst(layouts: List<TextInputLayout>): Boolean {
        val first = layouts.firstOrNull { it.error != null && it.isShown } ?: return false
        (first.editText ?: first).requestFocus()
        return true
    }
}
