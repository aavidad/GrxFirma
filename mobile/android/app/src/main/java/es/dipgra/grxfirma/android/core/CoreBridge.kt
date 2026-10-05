// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.core

import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.SignedOutput
import es.dipgra.grxfirma.android.model.VerificationSummary
import es.dipgra.grxfirma.android.model.SignatureInspection
import es.dipgra.grxfirma.android.model.BatchItemResult
import es.dipgra.grxfirma.android.model.HashCheck
import es.dipgra.grxfirma.android.model.HashOutput
import es.dipgra.grxfirma.android.model.ProtectionRequest
import es.dipgra.grxfirma.android.model.CsvLegend
import es.dipgra.grxfirma.android.model.EniCatalogs
import es.dipgra.grxfirma.android.model.EniDocument
import es.dipgra.grxfirma.android.model.EniRequest
import es.dipgra.grxfirma.android.model.EniValidation
import es.dipgra.grxfirma.android.model.VeriFactuReport
import es.dipgra.grxfirma.android.model.CertificateDetails
import es.dipgra.grxfirma.android.model.EngineDiagnostics
import es.dipgra.grxfirma.android.model.RevocationCheck
import es.dipgra.grxfirma.android.model.TsaProbe
import es.dipgra.grxfirma.android.model.UpdateCheck
import es.dipgra.grxfirma.android.model.VeriFactuQr
import es.dipgra.grxfirma.android.model.BatchItemInput
import es.dipgra.grxfirma.android.model.EniFileRequest
import es.dipgra.grxfirma.android.model.EniFileResult

data class CoreReadiness(
    val available: Boolean,
    val code: String,
    val detail: String,
)

interface CoreBridge {
    val engineVersion: String get() = ""
    val readiness: CoreReadiness

    fun selectCertificate(): CertificateSummary

    fun importCertificate(data: ByteArray, password: CharArray): CertificateSummary

    fun sign(
        document: LoadedFile,
        format: String,
        certificateId: String,
        options: Map<String, String> = emptyMap(),
        action: String = "sign",
    ): SignedOutput

    fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray

    fun verify(document: LoadedFile, original: LoadedFile? = null): VerificationSummary

    fun inspectSignature(document: LoadedFile): SignatureInspection {
        val report = verify(document)
        return SignatureInspection(report.signers.isNotEmpty() || report.signerSummaries.isNotEmpty(), report.format.lowercase())
    }

    fun clearSession()

    /** Huellas, protección y lote solo se ofrecen si el AAR los declara. */
    val toolsAvailable: Boolean get() = false

    fun createHash(document: LoadedFile, algorithm: String, format: String): HashOutput = toolsUnavailable()

    fun checkHash(document: LoadedFile, hashFile: LoadedFile): HashCheck = toolsUnavailable()

    /** [secret] es la clave de EncryptedData en Base64; se borra siempre. */
    fun protect(document: LoadedFile, request: ProtectionRequest, secret: CharArray?): SignedOutput {
        secret?.fill('\u0000')
        toolsUnavailable()
    }

    /** [secret] es la clave de EncryptedData en Base64; se borra siempre. */
    fun unprotect(document: LoadedFile, secret: CharArray?): SignedOutput {
        secret?.fill('\u0000')
        toolsUnavailable()
    }

    fun signBatch(
        documents: List<LoadedFile>,
        format: String,
        certificateId: String,
        options: Map<String, String>,
    ): List<BatchItemResult> = toolsUnavailable()

    /** Formatos de firma que declara el AAR; uno antiguo solo trae los tres básicos. */
    val signingFormats: List<String> get() = SignatureFormats.BASIC

    /** Servicios de documentos opcionales que el AAR declara y enlaza. */
    val documentServices: Set<String> get() = emptySet()

    fun validateVeriFactu(records: List<LoadedFile>): VeriFactuReport = toolsUnavailable()

    fun createEniDocument(signature: LoadedFile, original: LoadedFile?, request: EniRequest): EniDocument =
        toolsUnavailable()

    fun validateEni(document: LoadedFile): EniValidation = toolsUnavailable()

    /** Sin el método del AAR se usan los códigos NTI conocidos por la app. */
    fun eniCatalogs(): EniCatalogs = SignatureFormats.DEFAULT_ENI_CATALOGS

    fun csvLegend(code: String, url: String, text: String): CsvLegend = toolsUnavailable()

    /** Servicios de la tercera oleada que el AAR declara y enlaza. */
    val platformServices: Set<String> get() = emptySet()

    fun certificateDetails(): CertificateDetails = toolsUnavailable()

    /** Consulta OCSP/CRL; solo tras pulsar «Comprobar en línea». */
    fun checkCertificateRevocation(certificateId: String): RevocationCheck = toolsUnavailable()

    fun diagnostics(): EngineDiagnostics = toolsUnavailable()

    /** Pide un sello de tiempo de prueba a la TSA indicada. */
    fun probeTimestampAuthority(url: String): TsaProbe = toolsUnavailable()

    fun readVeriFactuQr(url: String): VeriFactuQr = toolsUnavailable()

    /** Devuelve el JSON de la AEAT; solo tras pulsar «Cotejar con la AEAT». */
    fun queryVeriFactuQr(url: String): String = toolsUnavailable()

    fun checkUpdate(currentVersion: String): UpdateCheck = toolsUnavailable()

    // --- Cuarta oleada: expediente ENI y lote con operación y opciones por documento ---

    /** Capacidades declaradas por el contrato (véase Wave4Capabilities). */
    val capabilities: Set<String> get() = emptySet()

    fun createEniFile(documents: List<LoadedFile>, certificateId: String, request: EniFileRequest): EniFileResult =
        toolsUnavailable()

    fun signBatchItems(items: List<BatchItemInput>, certificateId: String, options: Map<String, String>): List<BatchItemResult> =
        toolsUnavailable()
}

/** Nombres de servicio del contrato para la tercera oleada. */
object PlatformServices {
    const val CERTIFICATE_DETAILS = "certificate_details"
    const val CERTIFICATE_ONLINE = "certificate_online_check"
    const val DIAGNOSTICS = "diagnostics"
    const val TSA_PROBE = "tsa_probe"
    const val VERIFACTU_QR_READ = "verifactu_qr_read"
    const val VERIFACTU_QR_QUERY = "verifactu_qr_query"
    const val UPDATE_CHECK = "update_check"
    const val VERIFY_REPORT_HTML = "verify_report_html"
    val ALL = listOf(CERTIFICATE_DETAILS, CERTIFICATE_ONLINE, DIAGNOSTICS, TSA_PROBE,
        VERIFACTU_QR_READ, VERIFACTU_QR_QUERY, UPDATE_CHECK, VERIFY_REPORT_HTML)
}

/** Nombres de servicio del contrato para las herramientas de documentos. */
object DocumentServices {
    const val VERIFACTU = "verifactu_validate"
    const val ENI_DOCUMENT = "eni_document"
    const val ENI_VALIDATE = "eni_validate"
    const val CSV_LEGEND = "csv_legend"
    const val ENI_FILE = "eni_file"
    val ALL = listOf(VERIFACTU, ENI_DOCUMENT, ENI_VALIDATE, CSV_LEGEND, ENI_FILE)
}

object SignatureFormats {
    val BASIC = listOf("cades", "pades", "xades")
    val ALL = listOf("cades", "pades", "xades", "xmldsig", "odf", "ooxml", "facturae", "asic-xades", "verifactu")
    val DEFAULT_ENI_CATALOGS = EniCatalogs(
        documentStates = listOf("EE01", "EE02", "EE03", "EE04", "EE99"),
        documentTypes = (1..20).map { "TD" + it.toString().padStart(2, '0') } + "TD99",
        fileStates = listOf("E01", "E02", "E03"),
    )
}

private fun toolsUnavailable(): Nothing = throw CoreUnavailableException("TOOLS_UNAVAILABLE")

interface ExternalIdentityBridge {
    fun installExternalIdentity(certificate: ByteArray, chain: List<ByteArray>, signDigest: (ByteArray, String) -> ByteArray): CertificateSummary
}

class CoreUnavailableException(message: String) : Exception(message)

class CoreContractException(message: String, cause: Throwable? = null) : Exception(message, cause)

class UnavailableCoreBridge(override val readiness: CoreReadiness) : CoreBridge {
    private fun unavailable(): Nothing = throw CoreUnavailableException(readiness.detail)

    override fun selectCertificate(): CertificateSummary = unavailable()

    override fun importCertificate(data: ByteArray, password: CharArray): CertificateSummary = unavailable()

    override fun sign(
        document: LoadedFile,
        format: String,
        certificateId: String,
        options: Map<String, String>,
        action: String,
    ): SignedOutput = unavailable()

    override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray = unavailable()

    override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary = unavailable()

    override fun clearSession() = Unit
}
