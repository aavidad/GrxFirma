// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.core

import es.dipgra.grxfirma.android.files.DocumentPolicy
import es.dipgra.grxfirma.android.model.BatchItemInput
import es.dipgra.grxfirma.android.model.EniFileRequest
import es.dipgra.grxfirma.android.model.EniFileResult
import es.dipgra.grxfirma.android.model.LoadedFile
import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject
import java.util.Base64

/**
 * Capacidades de la cuarta oleada que declara el contrato. No tienen método
 * propio: amplían lote y protección. Un AAR anterior no las declara y la app
 * mantiene el comportamiento previo.
 */
object Wave4Capabilities {
    const val BATCH_VISIBLE_SEAL = "batch_visible_seal"
    const val BATCH_COSIGN = "batch_cosign"
    const val EXTERNAL_BATCH = "external_signer_batch"
    const val EXTERNAL_PROTECT_SIGN = "external_signer_protect_sign"
    val ALL = listOf(BATCH_VISIBLE_SEAL, BATCH_COSIGN, EXTERNAL_BATCH, EXTERNAL_PROTECT_SIGN)
}

/** JSON de los servicios nuevos; mismas reglas de saneado que CoreJsonCodec. */
object Wave4Codec {
    private const val MAX_ISSUES_DOCUMENTS = 256

    fun capabilities(raw: String): Set<String> {
        val services = try { JSONObject(raw).optJSONObject("services") } catch (_: JSONException) { null }
            ?: return emptySet()
        if (services.optBoolean("remote_exchange", false)) return emptySet()
        return Wave4Capabilities.ALL.filter { services.optBoolean(it, false) }.toSet()
    }

    fun eniFileRequest(documents: List<LoadedFile>, certificateId: String, request: EniFileRequest): String =
        JSONObject()
            .put("certificate_id", certificateId)
            .put("documents", JSONArray(documents.map { document ->
                JSONObject()
                    .put("name", document.displayName)
                    .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
            }))
            .put("organs", JSONArray(request.organs))
            .put("classification", request.classification)
            .put("state", request.state)
            .apply {
                if (request.identifier.isNotBlank()) put("identifier", request.identifier)
                if (request.openingDate.isNotBlank()) put("opening_date", request.openingDate)
                if (request.interested.isNotEmpty()) put("interested", JSONArray(request.interested))
            }
            .toString()

    fun parseEniFile(raw: String): EniFileResult {
        val json = try { JSONObject(raw) } catch (error: JSONException) {
            throw CoreContractException("El núcleo devolvió una respuesta de expediente no válida.", error)
        }
        val documents = json.optInt("documents", 0).coerceIn(0, MAX_ISSUES_DOCUMENTS)
        val issues = CoreJsonCodec.parseIssues(json.optJSONArray("issues"))
        if (!json.optBoolean("ok", false)) {
            if (issues.isEmpty()) throw CoreContractException("ENI_FILE_ISSUES_MISSING")
            return EniFileResult(null, documents, issues)
        }
        val encoded = json.optString("content_base64")
        if (encoded.isBlank()) throw CoreContractException("Falta el campo obligatorio 'content_base64'.")
        return EniFileResult(CoreJsonCodec.decodeBounded(encoded, DocumentPolicy.MAX_SIGNED_OUTPUT_BYTES), documents, emptyList())
    }

    /** Cada documento lleva su formato, operación y opciones (sello visible del PDF). */
    fun batchItemsRequest(items: List<BatchItemInput>, certificateId: String, options: Map<String, String>): String =
        JSONObject()
            .put("certificate_id", certificateId)
            .put("options", JSONObject(options))
            .put("items", JSONArray(items.map { item ->
                JSONObject()
                    .put("name", item.file.displayName)
                    .put("content_base64", Base64.getEncoder().encodeToString(item.file.bytes))
                    .put("mime_type", item.file.mimeType)
                    .put("format", if (item.format == "auto") "" else item.format)
                    .put("action", item.action)
                    .apply { if (item.options.isNotEmpty()) put("options", JSONObject(item.options)) }
            }))
            .toString()
}
