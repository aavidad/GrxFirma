// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.core

import es.dipgra.grxfirma.android.files.DocumentPolicy
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.SignedOutput
import es.dipgra.grxfirma.android.model.VerificationSummary
import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject
import java.util.Base64

object CoreJsonCodec {
    const val CONTRACT_VERSION = 1

    fun signRequest(
        document: LoadedFile,
        format: String,
        certificateId: String,
        options: Map<String, String> = emptyMap(),
    ): String = JSONObject()
        .put("name", document.displayName)
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("mime_type", document.mimeType)
        .put("format", format)
        .put("action", "sign")
        .put("certificate_id", certificateId)
        .put("options", JSONObject(options))
        .toString()

    fun sealPreviewRequest(certificateId: String, options: Map<String, String>): String = JSONObject()
        .put("certificate_id", certificateId)
        .put("options", JSONObject(options))
        .toString()

    fun parseSealPreview(raw: String): ByteArray = decodeBounded(
        requiredText(parseObject(raw, "vista previa"), "image_base64", allowLong = true),
        32 * 1024 * 1024,
    )

    fun verifyRequest(document: LoadedFile, original: LoadedFile? = null): String = JSONObject()
        .put("name", document.displayName)
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("mime_type", document.mimeType)
        .apply {
            if (original != null) {
                put(
                    "original_content_base64",
                    Base64.getEncoder().encodeToString(original.bytes),
                )
            }
        }
        .toString()

    fun importCertificateRequest(data: ByteArray, password: CharArray): String = JSONObject()
        .put("data_base64", Base64.getEncoder().encodeToString(data))
        .put("password", password.concatToString())
        .toString()

    fun selectCertificateRequest(): String = JSONObject()
        .put("subject_filter", "")
        .put("issuer_filter", "")
        .put("solo_no_caducados", true)
        .toString()

    fun parseContract(raw: String) {
        val json = parseObject(raw, "contrato")
        requireField(json.optInt("contract_version", -1) == CONTRACT_VERSION) {
            "Versión de contrato mobile incompatible."
        }
        requireField(json.optString("platform") == "android") {
            "El AAR no declara la plataforma Android."
        }
        val services = json.optJSONObject("services")
            ?: throw CoreContractException("El AAR no declara sus servicios.")
        for (service in listOf("sign", "verify", "select_certificate", "import_certificate")) {
            requireField(services.optBoolean(service, false)) {
                "El servicio '$service' no está operativo en el AAR."
            }
        }
    }

    fun parseCertificate(raw: String): CertificateSummary {
        val json = parseObject(raw, "certificado")
        return CertificateSummary(
            id = requiredText(json, "certificate_id"),
            subject = requiredText(json, "subject"),
            issuer = requiredText(json, "issuer"),
            fingerprint = cleanText(json.optString("fingerprint")),
        )
    }

    fun parseSigned(raw: String, originalName: String): SignedOutput {
        val json = parseObject(raw, "firma")
        val format = requiredText(json, "format")
        val algorithm = requiredText(json, "algorithm")
        val encoded = requiredText(json, "signed_content_base64", allowLong = true)
        val bytes = decodeBounded(encoded, DocumentPolicy.MAX_SIGNED_OUTPUT_BYTES)
        val extension = when (format.lowercase()) {
            "pades" -> "pdf"
            "xades", "xmldsig" -> "xml"
            else -> "p7s"
        }
        val base = originalName.substringBeforeLast('.').ifBlank { "documento" }.take(120)
        val mime = when (extension) {
            "pdf" -> "application/pdf"
            "xml" -> "application/xml"
            else -> "application/pkcs7-signature"
        }
        return SignedOutput(bytes, "$base-firmado.$extension", mime, format, algorithm)
    }

    fun parseVerification(raw: String): VerificationSummary {
        val json = parseObject(raw, "verificación")
        return VerificationSummary(
            valid = json.optBoolean("valid", false),
            reason = cleanText(json.optString("reason")),
            details = json.optJSONArray("details").toCleanStrings(),
            signers = json.optJSONArray("signers").toCleanStrings(),
            format = cleanText(json.optString("format")),
            coverage = cleanText(json.optString("coverage")),
            integrityStatus = verificationStatus(json.optString("integrity_status")),
            certificateStatus = verificationStatus(json.optString("certificate_status")),
            trustStatus = verificationStatus(json.optString("trust_status")),
            revocationMode = revocationMode(json.optString("revocation_mode")),
            warnings = json.optJSONArray("warnings").toCleanStrings(),
            errors = json.optJSONArray("errors").toCleanStrings(),
        )
    }

    private fun verificationStatus(raw: String): String = when (raw.trim().lowercase()) {
        "valid", "invalid", "warning", "unknown" -> raw.trim().lowercase()
        else -> "unknown"
    }

    private fun revocationMode(raw: String): String = when (raw.trim().lowercase()) {
        "embedded_evidence_only", "online", "not_available" -> raw.trim().lowercase()
        else -> "not_available"
    }

    private fun decodeBounded(encoded: String, maximumBytes: Int): ByteArray {
        val maximumEncodedLength = ((maximumBytes.toLong() + 2L) / 3L) * 4L
        if (encoded.length > maximumEncodedLength) {
            throw CoreContractException("El resultado del núcleo supera el límite permitido.")
        }
        return try {
            Base64.getDecoder().decode(encoded).also {
                if (it.isEmpty() || it.size > maximumBytes) {
                    it.fill(0)
                    throw CoreContractException("El resultado del núcleo tiene un tamaño no válido.")
                }
            }
        } catch (error: IllegalArgumentException) {
            throw CoreContractException("El núcleo devolvió una firma con base64 no válido.", error)
        }
    }

    private fun parseObject(raw: String, label: String): JSONObject = try {
        JSONObject(raw)
    } catch (error: JSONException) {
        throw CoreContractException("El núcleo devolvió una respuesta de $label no válida.", error)
    }

    private fun requiredText(json: JSONObject, key: String, allowLong: Boolean = false): String {
        val raw = json.optString(key)
        if (raw.isBlank()) throw CoreContractException("Falta el campo obligatorio '$key'.")
        return if (allowLong) raw else cleanText(raw)
    }

    private fun cleanText(value: String): String = value
        .filter { it == '\n' || it == '\t' || !it.isISOControl() }
        .trim()
        .take(500)

    private fun JSONArray?.toCleanStrings(): List<String> {
        if (this == null) return emptyList()
        return buildList {
            for (index in 0 until minOf(length(), 64)) {
                val value = cleanText(optString(index))
                if (value.isNotBlank()) add(value)
            }
        }
    }

    private inline fun requireField(condition: Boolean, message: () -> String) {
        if (!condition) throw CoreContractException(message())
    }
}
