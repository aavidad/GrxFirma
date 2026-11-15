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
    // firma.sha1_corresponde y firma.lt.crl_no_aplicable: avisos del motor
    // tras firmar (lote 17b, 3b).
    private val CATALOGUE_KEY = Regex("""(verifactu|eni\.validacion|eni\.codigo|csv\.error|firma\.cofirma_sin_original|firma\.sha1_corresponde|firma\.lt\.crl_no_aplicable|factura\.revision\.incidencias)\.[A-Za-z0-9_]{1,40}""")
	private val BLOCK_MARKER = Regex("""\[([a-z0-9_]{1,40}); (excepciones\.[a-z0-9_]{1,40}\.bloqueo)\]$""")
	private val EXCEPTION_IDS = setOf(
		"cms_cifrado_heredado", "cms_cbc", "cms_rsa_v15_crear", "cms_rsa_v15_leer", "cms_aes_ecb",
		"afp_aad_ambigua", "alto_clave_fichero_crear", "alto_clave_fichero_leer", "ca_clave_fichero",
		"sha1_crear", "sha1_leer", "sha1_conservar", "xml_c14n_antigua", "pdf_cifrado_heredado",
		"rsa_corta", "des_intercambio", "tsa_http", "proxy_http", "trifasica_http",
		"ca_instalacion_automatica", "cofirma_sin_original", "origen_vacio", "rutas_directas",
		"aprobacion_automatica", "llamante_desarrollo", "pkcs11_experimental"
	)

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

    fun isClosed(key: String): Boolean {
		if (key in setOf("excepciones.aceptacion_requerida", "excepciones.version_aceptacion_invalida", "excepciones.canal_no_local", "excepciones.activar_continuar", "excepciones.cancelar", "excepciones.mas_informacion", "excepciones.reintento_caducado", "excepciones.reintento_no_disponible")) return true
		if (CATALOGUE_KEY.matches(key)) return true
		val parts = key.split('.')
		return parts.size == 3 && parts[0] == "excepciones" && parts[1] in EXCEPTION_IDS &&
			parts[2] in setOf("aviso", "bloqueo", "ajuste", "titulo", "riesgo", "aceptacion", "ayuda", "organizacion")
	}

	fun blockKey(message: String): String? {
		val marker = BLOCK_MARKER.find(message) ?: return null
		val id = marker.groupValues[1]
		val key = marker.groupValues[2]
		if (id !in EXCEPTION_IDS || key != "excepciones.$id.bloqueo") return null
        val organizationKey = "excepciones.$id.organizacion"
        if (message.contains("[$id; $organizationKey]")) return organizationKey
        return key
	}

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
