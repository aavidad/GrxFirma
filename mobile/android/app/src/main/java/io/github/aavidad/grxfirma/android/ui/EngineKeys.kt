// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import androidx.annotation.StringRes
import io.github.aavidad.grxfirma.android.R

/**
 * Claves cerradas que devuelve la fachada. Las del catálogo de escritorio se
 * traducen con los catálogos empaquetados (EngineText); las propias de
 * Android, con recursos. Nada fuera de esta lista llega a la pantalla.
 */
object EngineKeys {
    private val CATALOGUE_KEY = Regex("""(verifactu|eni\.validacion|eni\.codigo|csv\.error)\.[A-Za-z0-9_]{1,40}""")

    private val LOCAL = mapOf(
        "eni.error.unsigned_pdf" to R.string.eni_error_unsigned_pdf,
        "eni.error.signature_mismatch" to R.string.eni_error_signature_mismatch,
        "eni.error.explicit_cades" to R.string.eni_error_explicit_cades,
        "eni.error.unrecognized" to R.string.eni_error_unrecognized,
        "eni.error.content_format" to R.string.eni_error_content_format,
        "eni.error.origin" to R.string.eni_error_origin,
        // Sesión llena y QR desde imagen: textos propios de Android (el
        // catálogo de escritorio habla también de PDF, que aquí no se lee).
        "session.full" to R.string.error_session_full,
        "verifactu.qr_image" to R.string.qr_error_image,
        "verifactu.qr_not_found" to R.string.qr_error_not_found,
        "verifactu.qr_timeout" to R.string.qr_error_timeout,
    )

    fun isClosed(key: String): Boolean = CATALOGUE_KEY.matches(key)

    @StringRes
    fun localResource(key: String): Int? = LOCAL[key]

    /** Campo del formulario CSV al que se refiere cada error. */
    fun csvField(key: String): String? = when (key) {
        "csv.error.code_missing", "csv.error.code_invalid" -> "code"
        "csv.error.url_missing", "csv.error.url_invalid" -> "url"
        "csv.error.text_invalid" -> "text"
        else -> null
    }
}
