// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import androidx.annotation.StringRes
import io.github.aavidad.grxfirma.android.R

/**
 * Reglas de formato iguales a las de la fachada móvil (mobilebind/formats.go):
 * acciones, perfiles y detección automática por extensión, MIME y raíz XML.
 */
object FormatPolicy {
    data class Rule(val actions: Set<String>, val profiles: Set<String>, val rsaOnly: Boolean)

    private val ALL_ACTIONS = setOf("sign", "cosign", "countersign")
    private val SIGN_COSIGN = setOf("sign", "cosign")
    private val SIGN = setOf("sign")
    private val BASELINE = setOf("baseline")

    val RULES: Map<String, Rule> = linkedMapOf(
        "pades" to Rule(SIGN_COSIGN, setOf("baseline", "t", "lt"), false),
        "cades" to Rule(ALL_ACTIONS, setOf("baseline", "t", "lt", "lta"), false),
        "xades" to Rule(ALL_ACTIONS, setOf("baseline", "t"), true),
        "xmldsig" to Rule(SIGN_COSIGN, BASELINE, true),
        "odf" to Rule(SIGN_COSIGN, BASELINE, true),
        "ooxml" to Rule(SIGN_COSIGN, BASELINE, true),
        "facturae" to Rule(SIGN, BASELINE, true),
        "asic-xades" to Rule(SIGN, BASELINE, true),
        "verifactu" to Rule(SIGN, BASELINE, true),
    )

    /** Orden del desplegable: el de siempre y después los formatos de escritorio. */
    val MENU_ORDER = RULES.keys.toList()

    private val ODF_EXTENSIONS = setOf("odt", "ods", "odp", "odg", "odf")
    private val OOXML_EXTENSIONS = setOf(
        "docx", "docm", "dotx", "dotm", "xlsx", "xlsm", "xltx", "xltm", "pptx", "pptm", "ppsx", "ppsm",
    )

    fun supported(format: String, action: String, profile: String): Boolean {
        val rule = RULES[format] ?: return false
        return action in rule.actions && profile in rule.profiles
    }

    /** Los formatos que solo generan B no reciben la TSA. */
    fun acceptsTimestamp(format: String): Boolean = RULES[format]?.profiles?.contains("t") == true

    fun requiresRsa(format: String): Boolean = RULES[format]?.rsaOnly == true

    /** Formato de «automático». [head] es el inicio del contenido, si ya está leído. */
    fun detect(name: String, mimeType: String, head: ByteArray? = null): String {
        val lowerName = name.trim().lowercase()
        val extension = lowerName.substringAfterLast('.', "")
        val mime = mimeType.substringBefore(';').trim().lowercase()
        return when {
            mime == "application/pdf" || extension == "pdf" -> "pades"
            extension in OOXML_EXTENSIONS || mime.startsWith("application/vnd.openxmlformats-officedocument.") -> "ooxml"
            extension in ODF_EXTENSIONS || mime.startsWith("application/vnd.oasis.opendocument.") -> "odf"
            extension == "asics" || mime == "application/vnd.etsi.asic-s+zip" -> "asic-xades"
            extension == "dsig" || extension == "xmlsig" -> "xmldsig"
            extension == "xml" || extension == "xsig" || mime.contains("xml") ->
                if (lowerName.contains("facturae") || isFacturaE(head)) "facturae" else "xades"
            else -> "cades"
        }
    }

    private val SKIPPED = Regex("""^\s*(?:<\?[\s\S]*?\?>|<!--[\s\S]*?-->|<!DOCTYPE[^>]*>)""")
    private val FIRST_ELEMENT = Regex("""^\s*<(?:[A-Za-z_][\w.-]*:)?([A-Za-z_][\w.-]*)""")

    /** Mismo criterio que el motor: la raíz del XML se llama Facturae. */
    fun isFacturaE(head: ByteArray?): Boolean {
        if (head == null || head.isEmpty()) return false
        var text = String(head, 0, minOf(head.size, 4096), Charsets.UTF_8).removePrefix("\uFEFF")
        while (true) {
            val skipped = SKIPPED.find(text) ?: break
            text = text.substring(skipped.range.last + 1)
        }
        return FIRST_ELEMENT.find(text)?.groupValues?.get(1) == "Facturae"
    }

    @StringRes
    fun label(format: String): Int = when (format) {
        "pades" -> R.string.format_pades
        "cades" -> R.string.format_cades
        "xades" -> R.string.format_xades
        "xmldsig" -> R.string.format_xmldsig
        "odf" -> R.string.format_odf
        "ooxml" -> R.string.format_ooxml
        "facturae" -> R.string.format_facturae
        "asic-xades" -> R.string.format_asic_xades
        "verifactu" -> R.string.format_verifactu
        else -> R.string.format_auto
    }
}
