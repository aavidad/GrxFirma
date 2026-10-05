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
import es.dipgra.grxfirma.android.model.SignerSummary
import es.dipgra.grxfirma.android.model.BatchItemResult
import es.dipgra.grxfirma.android.model.HashCheck
import es.dipgra.grxfirma.android.model.HashOutput
import es.dipgra.grxfirma.android.model.ProtectionRequest
import es.dipgra.grxfirma.android.model.CsvLegend
import es.dipgra.grxfirma.android.model.EngineIssue
import es.dipgra.grxfirma.android.model.EniCatalogs
import es.dipgra.grxfirma.android.model.EniDocument
import es.dipgra.grxfirma.android.model.EniRequest
import es.dipgra.grxfirma.android.model.EniValidation
import es.dipgra.grxfirma.android.model.VeriFactuRecord
import es.dipgra.grxfirma.android.model.VeriFactuReport
import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject
import java.util.Base64

object CoreJsonCodec {
    const val CONTRACT_VERSION = 2
    const val MAX_HASH_FILE_BYTES = 4 * 1024
    private const val MAX_REPORT_ITEMS = 256
    private val TOOL_SERVICES = listOf("process_batch", "hash", "protect", "unprotect", "protect_sign")

    fun signRequest(
        document: LoadedFile,
        format: String,
        certificateId: String,
        options: Map<String, String> = emptyMap(),
        action: String = "sign",
    ): String = JSONObject()
        .put("name", document.displayName)
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("mime_type", document.mimeType)
        .put("format", format)
        .put("action", action)
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

    fun selectCertificateRequest(): String = JSONObject()
        .put("subject_filter", "")
        .put("issuer_filter", "")
        .put("solo_no_caducados", true)
        .toString()

    fun engineVersion(raw: String): String = cleanText(parseObject(raw, "contract").optString("engine_version"))

    fun parseContract(raw: String) {
        val json = parseObject(raw, "contrato")
        requireField(json.optInt("contract_version", -1) == CONTRACT_VERSION) {
            "Versión de contrato mobile incompatible."
        }
        requireField(json.optString("platform") == "android") {
            "El AAR no declara la plataforma Android."
        }
        val signing = json.optJSONObject("signing")
            ?: throw CoreContractException("SIGNING_CAPABILITIES_MISSING")
        val actions = signing.optJSONArray("actions").toCleanStrings()
        requireField(actions.containsAll(listOf("sign", "cosign", "countersign"))) { "SIGNING_ACTIONS_MISSING" }
        requireField(signing.optJSONObject("profiles_by_format")?.optJSONArray("CAdES")
            .toCleanStrings().containsAll(listOf("baseline", "t", "lt", "lta"))) { "SIGNING_PROFILES_MISSING" }
        val services = json.optJSONObject("services")
            ?: throw CoreContractException("El AAR no declara sus servicios.")
        for (service in listOf("sign", "verify", "select_certificate", "import_certificate", "external_signer")) {
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

    fun parseSigned(
        raw: String,
        originalName: String,
        outputName: (String, String) -> String = { base, extension -> "$base.$extension" },
        originalMime: String = "application/octet-stream",
    ): SignedOutput {
        val json = parseObject(raw, "firma")
        return signedOutput(json, originalName, outputName, originalMime)
    }

    private fun signedOutput(
        json: JSONObject,
        originalName: String,
        outputName: (String, String) -> String,
        originalMime: String,
    ): SignedOutput {
        val format = requiredText(json, "format")
        val algorithm = requiredText(json, "algorithm")
        val encoded = requiredText(json, "signed_content_base64", allowLong = true)
        val bytes = decodeBounded(encoded, DocumentPolicy.MAX_SIGNED_OUTPUT_BYTES)
        val (extension, mime) = signedFileType(format, originalName, originalMime)
        val base = originalName.substringBeforeLast('.').ifBlank { "document" }.take(120)
        return SignedOutput(bytes, outputName(base, extension), mime, format, algorithm)
    }

    /** ODF y OOXML conservan el tipo del documento: la firma va dentro. */
    fun signedFileType(format: String, originalName: String, originalMime: String): Pair<String, String> =
        when (format.lowercase()) {
            "pades" -> "pdf" to "application/pdf"
            "xades", "xmldsig", "facturae", "verifactu" -> "xml" to "application/xml"
            "asic-xades" -> "asics" to "application/vnd.etsi.asic-s+zip"
            "odf", "ooxml" -> {
                val extension = originalName.substringAfterLast('.', "").lowercase()
                    .takeIf { it.matches(Regex("[a-z]{3,4}")) } ?: if (format.equals("odf", true)) "odt" else "docx"
                val mime = originalMime.takeIf {
                    it.startsWith("application/vnd.oasis.opendocument.") ||
                        it.startsWith("application/vnd.openxmlformats-officedocument.")
                } ?: "application/octet-stream"
                extension to mime
            }
            else -> "p7s" to "application/pkcs7-signature"
        }

    fun parseVerification(raw: String): VerificationSummary {
        if (raw.length > 512 * 1024) throw CoreContractException("REPORT_TOO_LARGE")
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
            signerSummaries = json.optJSONArray("signer_summaries").let { items ->
                buildList {
                    if (items != null) for (i in 0 until minOf(items.length(), 64)) {
                        val item = items.optJSONObject(i) ?: continue
                        add(SignerSummary(cleanText(item.optString("id")), cleanText(item.optString("subject")),
                            cleanText(item.optString("issuer")), cleanText(item.optString("fingerprint"))))
                    }
                }
            },
            reportJson = json.toString(2),
        )
    }

    /** Las herramientas se habilitan solo si el contrato las declara todas. */
    fun toolsDeclared(raw: String): Boolean {
        val services = parseObject(raw, "contrato").optJSONObject("services") ?: return false
        return TOOL_SERVICES.all { services.optBoolean(it, false) } && !services.optBoolean("remote_exchange", false)
    }

    fun hashRequest(document: LoadedFile, algorithm: String, format: String): String = JSONObject()
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("algorithm", algorithm)
        .put("format", format)
        .toString()

    fun parseHash(raw: String): HashOutput {
        val json = parseObject(raw, "huella")
        val extension = requiredText(json, "extension")
        requireField(extension in listOf("hexhash", "hashb64", "hash")) { "HASH_EXTENSION_INVALID" }
        return HashOutput(
            algorithm = requiredText(json, "algorithm"),
            format = requiredText(json, "format"),
            hash = requiredText(json, "hash"),
            bytes = decodeBounded(requiredText(json, "output_base64", allowLong = true), MAX_HASH_FILE_BYTES),
            extension = extension,
        )
    }

    fun hashCheckRequest(document: LoadedFile, hashFile: LoadedFile): String = JSONObject()
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("hash_file_base64", Base64.getEncoder().encodeToString(hashFile.bytes))
        .put("hash_file_name", hashFile.displayName)
        .toString()

    fun parseHashCheck(raw: String): HashCheck {
        val json = parseObject(raw, "comprobación de huella")
        return HashCheck(
            valid = json.optBoolean("valid", false),
            algorithm = requiredText(json, "algorithm"),
            expected = requiredText(json, "expected_hash"),
            actual = requiredText(json, "actual_hash"),
        )
    }

    fun protectRequest(document: LoadedFile, request: ProtectionRequest): String = JSONObject()
        .put("name", document.displayName)
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("mime_type", document.mimeType)
        .put("container", request.container)
        .apply {
            if (request.recipients.isNotEmpty()) {
                put("recipients", JSONArray(request.recipients.map {
                    JSONObject().put("certificate_base64", Base64.getEncoder().encodeToString(it))
                }))
            }
            if (request.includeSessionCertificate) put("include_session_certificate", true)
            if (request.sign) put("sign", true).put("certificate_id", request.certificateId)
        }
        .toString()

    fun unprotectRequest(document: LoadedFile): String = JSONObject()
        .put("name", document.displayName)
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .put("mime_type", document.mimeType)
        .toString()

    /** Respuesta de proteger o desproteger: un fichero listo para guardar por SAF. */
    fun parseFileOutput(raw: String, label: String, fallbackName: String): SignedOutput {
        val json = parseObject(raw, label)
        val bytes = decodeBounded(requiredText(json, "content_base64", allowLong = true), DocumentPolicy.MAX_SIGNED_OUTPUT_BYTES)
        val mime = cleanText(json.optString("mime_type")).takeIf {
            it.matches(Regex("[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*"))
        } ?: "application/octet-stream"
        return SignedOutput(
            bytes = bytes,
            displayName = DocumentPolicy.sanitizeDisplayName(json.optString("name"), fallbackName),
            mimeType = mime,
            format = cleanText(json.optString("container")),
            algorithm = "",
        )
    }

    fun batchRequest(
        documents: List<LoadedFile>,
        format: String,
        certificateId: String,
        options: Map<String, String>,
    ): String = JSONObject()
        .put("certificate_id", certificateId)
        .put("options", JSONObject(options))
        .put("items", JSONArray(documents.map { document ->
            JSONObject()
                .put("name", document.displayName)
                .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
                .put("mime_type", document.mimeType)
                .put("format", if (format == "auto") "" else format)
                .put("action", "sign")
        }))
        .toString()

    fun parseBatch(
        raw: String,
        documents: List<LoadedFile>,
        outputName: (String, String) -> String = { base, extension -> "$base.$extension" },
    ): List<BatchItemResult> {
        val items = parseObject(raw, "lote").optJSONArray("items")
            ?: throw CoreContractException("BATCH_ITEMS_MISSING")
        requireField(items.length() == documents.size) { "BATCH_ITEMS_MISMATCH" }
        val results = ArrayList<BatchItemResult>(documents.size)
        try {
            for (index in documents.indices) {
                val item = items.optJSONObject(index) ?: throw CoreContractException("BATCH_ITEM_INVALID")
                requireField(item.optInt("index", -1) == index) { "BATCH_ITEM_ORDER" }
                val name = documents[index].displayName
                val output = if (item.optBoolean("ok", false)) {
                    signedOutput(item, name, outputName, documents[index].mimeType)
                } else null
                results += BatchItemResult(name, output)
            }
        } catch (error: Exception) {
            results.forEach { it.output?.bytes?.fill(0) }
            throw error
        }
        return results
    }

    /** Formatos del contrato; un AAR sin la lista solo trae los tres básicos. */
    fun signingFormats(raw: String): List<String> {
        val declared = parseObject(raw, "contrato").optJSONObject("signing")?.optJSONArray("formats")
            .toCleanStrings().map { it.lowercase() }
        if (declared.isEmpty()) return SignatureFormats.BASIC
        return SignatureFormats.ALL.filter { it in declared }
    }

    fun documentServices(raw: String): Set<String> {
        val services = parseObject(raw, "contrato").optJSONObject("services") ?: return emptySet()
        if (services.optBoolean("remote_exchange", false)) return emptySet()
        return DocumentServices.ALL.filter { services.optBoolean(it, false) }.toSet()
    }

    fun veriFactuRequest(records: List<LoadedFile>): String = JSONObject()
        .put("files", JSONArray(records.map {
            JSONObject().put("name", it.displayName)
                .put("content_base64", Base64.getEncoder().encodeToString(it.bytes))
        }))
        .toString()

    fun parseVeriFactu(raw: String): VeriFactuReport {
        if (raw.length > 4 * 1024 * 1024) throw CoreContractException("REPORT_TOO_LARGE")
        val json = parseObject(raw, "Veri*Factu")
        val items = json.optJSONArray("records") ?: throw CoreContractException("VERIFACTU_RECORDS_MISSING")
        return VeriFactuReport(
            valid = json.optBoolean("valid", false),
            errors = json.optInt("errors", 0).coerceAtLeast(0),
            warnings = json.optInt("warnings", 0).coerceAtLeast(0),
            records = buildList {
                for (index in 0 until minOf(items.length(), MAX_REPORT_ITEMS)) {
                    val item = items.optJSONObject(index) ?: continue
                    add(VeriFactuRecord(
                        file = cleanText(item.optString("file")),
                        type = cleanText(item.optString("type")),
                        hash = cleanText(item.optString("hash")),
                        calculatedHash = cleanText(item.optString("calculated_hash")),
                        previousHash = cleanText(item.optString("previous_hash")),
                        signed = item.optBoolean("signed", false),
                        valid = item.optBoolean("valid", false),
                        issues = parseIssues(item.optJSONArray("issues")),
                    ))
                }
            },
        )
    }

    fun eniDocumentRequest(signature: LoadedFile, original: LoadedFile?, request: EniRequest): String = JSONObject()
        .put("signature_base64", Base64.getEncoder().encodeToString(signature.bytes))
        .apply { if (original != null) put("original_base64", Base64.getEncoder().encodeToString(original.bytes)) }
        .put("organs", JSONArray(request.organs))
        .put("origin", request.origin)
        .put("state", request.state)
        .put("document_type", request.documentType)
        .apply {
            if (request.identifier.isNotBlank()) put("identifier", request.identifier)
            if (request.sourceIdentifier.isNotBlank()) put("source_identifier", request.sourceIdentifier)
            if (request.captureDate.isNotBlank()) put("capture_date", request.captureDate)
            if (request.contentFormat.isNotBlank()) put("content_format", request.contentFormat)
        }
        .toString()

    fun parseEniDocument(raw: String): EniDocument {
        val json = parseObject(raw, "ENI")
        val type = cleanText(json.optString("signature_type"))
        requireField(type.matches(Regex("TF0[2-6]"))) { "ENI_SIGNATURE_TYPE_INVALID" }
        return EniDocument(
            decodeBounded(requiredText(json, "content_base64", allowLong = true), DocumentPolicy.MAX_SIGNED_OUTPUT_BYTES),
            type,
        )
    }

    fun eniValidateRequest(document: LoadedFile): String = JSONObject()
        .put("content_base64", Base64.getEncoder().encodeToString(document.bytes))
        .toString()

    fun parseEniValidation(raw: String): EniValidation {
        val json = parseObject(raw, "ENI")
        return EniValidation(json.optBoolean("valid", false), parseIssues(json.optJSONArray("issues")))
    }

    fun parseEniCatalogs(raw: String): EniCatalogs {
        val json = parseObject(raw, "ENI")
        fun codes(key: String, pattern: Regex, fallback: List<String>): List<String> =
            json.optJSONArray(key).toCleanStrings().filter { it.matches(pattern) }.ifEmpty { fallback }
        val defaults = SignatureFormats.DEFAULT_ENI_CATALOGS
        return EniCatalogs(
            documentStates = codes("document_states", Regex("EE[0-9]{2}"), defaults.documentStates),
            documentTypes = codes("document_types", Regex("TD[0-9]{2}"), defaults.documentTypes),
            fileStates = codes("file_states", Regex("E[0-9]{2}"), defaults.fileStates),
        )
    }

    fun csvLegendRequest(code: String, url: String, text: String): String = JSONObject()
        .put("csv", code)
        .put("csv_url", url)
        .apply { if (text.isNotBlank()) put("csv_text", text) }
        .toString()

    fun parseCsvLegend(raw: String): CsvLegend {
        val json = parseObject(raw, "CSV")
        val url = requiredText(json, "url")
        val scheme = try { java.net.URI(url).scheme.orEmpty() } catch (_: java.net.URISyntaxException) { "" }
        requireField(scheme.equals("https", ignoreCase = true)) { "CSV_URL_INVALID" }
        return CsvLegend(url, requiredText(json, "text"))
    }

    internal fun parseIssues(items: JSONArray?): List<EngineIssue> = buildList {
        if (items != null) for (index in 0 until minOf(items.length(), MAX_REPORT_ITEMS)) {
            val item = items.optJSONObject(index) ?: continue
            add(EngineIssue(cleanText(item.optString("field")), cleanText(item.optString("key")),
                cleanText(item.optString("level")).ifBlank { "error" }))
        }
    }

    private fun verificationStatus(raw: String): String = when (raw.trim().lowercase()) {
        "valid", "invalid", "warning", "unknown" -> raw.trim().lowercase()
        else -> "unknown"
    }

    private fun revocationMode(raw: String): String = when (raw.trim().lowercase()) {
        "embedded_evidence_only", "online", "not_available" -> raw.trim().lowercase()
        else -> "not_available"
    }

    internal fun decodeBounded(encoded: String, maximumBytes: Int): ByteArray {
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
