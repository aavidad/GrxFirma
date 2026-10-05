// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import java.util.Locale

/** Marca el idioma activo aunque Android lo guarde con región («es-ES»). */
object LanguageTags {
    fun indexFor(current: String, tags: List<String>): Int {
        val first = current.substringBefore(',').trim()
        if (first.isEmpty()) return 0
        tags.indexOf(first).takeIf { it >= 0 }?.let { return it }
        val locale = Locale.forLanguageTag(first)
        if (locale.language == "ca" && locale.variant.equals("valencia", ignoreCase = true)) {
            return tags.indexOf("ca-ES-valencia").coerceAtLeast(0)
        }
        return tags.indexOf(locale.language).coerceAtLeast(0)
    }
}
