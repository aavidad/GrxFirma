// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.core

import org.json.JSONArray
import org.json.JSONObject

/** Identificadores cerrados; las claves recibidas nunca eligen texto arbitrario. */
object UnsafeExceptions {
    val ids = listOf(
        "cms_cifrado_heredado", "cms_cbc", "cms_rsa_v15_crear", "cms_rsa_v15_leer", "cms_aes_ecb",
        "afp_aad_ambigua", "alto_clave_fichero_crear", "alto_clave_fichero_leer", "ca_clave_fichero",
        "sha1_crear", "sha1_leer", "sha1_conservar", "xml_c14n_antigua", "pdf_cifrado_heredado",
        "rsa_corta", "des_intercambio", "tsa_http", "proxy_http", "trifasica_http",
        "ca_instalacion_automatica", "cofirma_sin_original", "origen_vacio", "rutas_directas",
        "aprobacion_automatica", "llamante_desarrollo", "pkcs11_experimental",
    )

    fun parseList(raw: String): List<UnsafeExceptionState> {
        val array = JSONArray(raw)
        val items = (0 until array.length()).map { parseState(array.getJSONObject(it)) }
        if (items.size != ids.size || items.map { it.id }.toSet() != ids.toSet())
            throw CoreContractException("UNSAFE_CATALOGUE_INVALID")
        return ids.map { id -> items.single { it.id == id } }
    }

    fun parseState(json: JSONObject): UnsafeExceptionState {
        val id = json.getString("id")
        if (id !in ids) throw CoreContractException("UNSAFE_ID_INVALID")
        return UnsafeExceptionState(
            id = id,
            enabled = json.opt("enabled") == true,
            fixed = json.opt("fixed") != false, // Un estado incompleto nunca permite cambiarlo.
            policyUnreadable = json.opt("policy_unreadable") == true,
            activable = json.opt("activable") == true,
            acceptanceVersion = version(json),
            acceptedAt = json.optJSONObject("last_acceptance")?.optString("accepted_at").orEmpty(),
        )
    }

    /** Tolera metadatos nuevos y ambos convenios de nombres, sin interpretar errores de texto. */
    fun parseBlock(raw: String?): UnsafeExceptionState? = try {
        if (raw == null || raw.length > 65536) null else {
            val envelope = JSONObject(raw)
            val json = envelope.optJSONObject("error") ?: envelope
            val block = json.optJSONObject("unsafeException") ?: json.optJSONObject("unsafe_exception") ?: json
            val id = block.optString("exceptionId").ifBlank { block.optString("id") }
            val blockKey = block.optString("blockKey").ifBlank { block.optString("block_key") }
            val settingKey = block.optString("settingKey").ifBlank { block.optString("setting_key") }
            if (id !in ids || blockKey != "excepciones.$id.bloqueo" ||
                settingKey != "excepciones.$id.ajuste" || block.opt("activable") !is Boolean) null
            else UnsafeExceptionState(id, activable = block.opt("activable") == true,
                fixed = block.opt("fixed") == true,
                policyUnreadable = block.opt("policy_unreadable") == true,
                acceptanceVersion = version(block))
        }
    } catch (_: Exception) { null }

    fun block(error: Throwable): UnsafeExceptionState? {
        var current: Throwable? = error
        repeat(5) {
            val cause = current ?: return null
            if (cause is UnsafeExceptionBlockedException) return cause.item
            parseBlock(cause.message)?.let { return it }
            current = cause.cause.takeUnless { it === cause }
        }
        return null
    }

    private fun version(json: JSONObject): String =
        json.optString("acceptanceVersion").ifBlank { json.optString("acceptance_version") }
            .takeIf { it.length in 1..80 && it.all { c -> c.isLetterOrDigit() || c in "._-" } }.orEmpty()

    fun activationRequest(item: UnsafeExceptionState, accepted: Boolean): String {
        require(accepted && item.canActivate)
        return JSONObject().put("id", item.id).put("accepted", true)
            .put("acceptanceVersion", item.acceptanceVersion).toString()
    }

    fun disableRequest(item: UnsafeExceptionState): String {
        require(item.canDisable)
        return JSONObject().put("id", item.id).put("enabled", false).toString()
    }
}

data class UnsafeExceptionState(
    val id: String,
    val enabled: Boolean = false,
    val fixed: Boolean = true,
    val policyUnreadable: Boolean = false,
    val activable: Boolean = false,
    val acceptanceVersion: String = "",
    val acceptedAt: String = "",
) {
    init { require(id in UnsafeExceptions.ids) }
    fun key(suffix: String) = "excepciones.$id.$suffix"
    val canActivate get() = activable && !fixed && !policyUnreadable && acceptanceVersion.isNotBlank()
    val canDisable get() = enabled && !fixed && !policyUnreadable
    val statusKey get() = "android.inseguro." + when {
        policyUnreadable -> "directiva_ilegible"
        fixed && enabled -> "fijada_activa"
        fixed -> "fijada_inactiva"
        enabled -> "activada"
        else -> "desactivada"
    }
}

class UnsafeExceptionBlockedException(val item: UnsafeExceptionState) : Exception(item.key("bloqueo"))
