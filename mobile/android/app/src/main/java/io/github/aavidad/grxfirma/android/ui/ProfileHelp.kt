// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R

/**
 * Ayuda del botón «?» junto al perfil de firma: un párrafo por perfil, en el
 * orden del desplegable, y al final el aviso de que T, LT y LTA necesitan red.
 */
object ProfileHelp {
    val PARAGRAPHS = listOf(
        R.string.profile_help_b,
        R.string.profile_help_t,
        R.string.profile_help_lt,
        R.string.profile_help_lta,
        R.string.profile_help_internet,
    )

    fun message(text: (Int) -> String): String = PARAGRAPHS.joinToString("\n\n") { text(it) }
}
