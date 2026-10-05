// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.core

/**
 * Caracteres que no se muestran nunca: los de control y los de formato
 * (marcas Bidi como U+202E, caracteres de ancho cero, U+FEFF). Con ellos un
 * nombre de fichero o una huella podrían aparentar otra cosa en pantalla.
 */
object DisplayText {
    fun hidden(char: Char): Boolean = char.isISOControl() || Character.getType(char) == Character.FORMAT.toInt()

    /** Quita los caracteres ocultos; conserva salto de línea y tabulador si se pide. */
    fun clean(value: String, keepLineBreaks: Boolean = false): String =
        value.filter { (keepLineBreaks && (it == '\n' || it == '\t')) || !hidden(it) }
}
