// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import org.json.JSONObject

// Certificate identities remain literal data. Engine diagnostics are translated
// from the same data catalogue as desktop, without adding prose to Kotlin.
internal object EngineText {
    private val catalogues = mutableMapOf<String, JSONObject>()

    @Synchronized
    fun resolve(context: Context, value: String): String {
        val locale = context.resources.configuration.locales[0]
        val language = if (locale.variant == "valencia") "va" else locale.language
        val supported = context.resources.getStringArray(io.github.aavidad.grxfirma.android.R.array.language_tags)
        val tag = if (language == "va" || language in supported) language else "es"
        val catalogue = catalogues.getOrPut(tag) {
            try { context.assets.open("locales/$tag.json").bufferedReader().use { JSONObject(it.readText()) } }
            catch (_: Exception) { JSONObject() }
        }
        return catalogue.optString(value, value)
    }
}
