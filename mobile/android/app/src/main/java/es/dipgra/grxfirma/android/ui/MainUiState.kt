// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.ui

import android.content.Context
import androidx.annotation.PluralsRes
import androidx.annotation.StringRes
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreContractException
import es.dipgra.grxfirma.android.core.CoreReadiness
import es.dipgra.grxfirma.android.core.CoreUnavailableException
import es.dipgra.grxfirma.android.core.DocumentServices
import es.dipgra.grxfirma.android.core.SignatureFormats
import es.dipgra.grxfirma.android.files.InvalidDocumentException
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.model.VerificationSummary
import es.dipgra.grxfirma.android.model.SignerSummary
import es.dipgra.grxfirma.android.core.PlatformServices
import es.dipgra.grxfirma.android.model.CertificateDetail
import es.dipgra.grxfirma.android.model.EngineDiagnostics
import es.dipgra.grxfirma.android.model.TsaProbe
import es.dipgra.grxfirma.android.model.UpdateCheck
import es.dipgra.grxfirma.android.model.VeriFactuQr
import es.dipgra.grxfirma.android.model.VeriFactuReport
import es.dipgra.grxfirma.android.settings.AppSettings

sealed interface UiText {
    data class Resource(@param:StringRes val id: Int, val arguments: List<Any> = emptyList()) : UiText
    data class Plural(@param:PluralsRes val id: Int, val count: Int, val arguments: List<Any> = emptyList()) : UiText
    data class Lines(val lines: List<UiText>) : UiText
    /** Clave cerrada del catálogo del motor (verifactu.*, eni.*, csv.error.*). */
    data class Engine(val key: String) : UiText
    /** Fecha RFC 3339 que se muestra en el idioma y zona horaria del dispositivo. */
    data class DateTime(val iso: String, val withTime: Boolean = false) : UiText
    data class Verification(
        val valid: Boolean,
        val reason: String = "",
        val format: String,
        val signerCount: Int,
        val integrityStatus: String,
        val certificateStatus: String,
        val trustStatus: String,
        val revocationMode: String,
        val warningCount: Int,
        val errorCount: Int,
        val coverage: String = "",
        val signers: List<String> = emptyList(),
        val signerSummaries: List<SignerSummary> = emptyList(),
        val details: List<String> = emptyList(),
        val warnings: List<String> = emptyList(),
        val errors: List<String> = emptyList(),
    ) : UiText {
        fun accredited(): Boolean = valid &&
            integrityStatus == "valid" &&
            certificateStatus == "valid" &&
            trustStatus == "valid"
    }
}

fun UiText.resolve(context: Context): String = when (this) {
    is UiText.Resource -> context.getString(id, *arguments.map { if (it is UiText) it.resolve(context) else it }.toTypedArray())
    is UiText.Plural -> context.resources.getQuantityString(id, count, count, *arguments.toTypedArray())
    is UiText.Lines -> lines.joinToString("\n") { it.resolve(context) }
    is UiText.DateTime -> try {
        val instant = java.time.OffsetDateTime.parse(iso).toInstant()
        val format = if (withTime) java.text.DateFormat.getDateTimeInstance(java.text.DateFormat.MEDIUM, java.text.DateFormat.SHORT)
        else java.text.DateFormat.getDateInstance(java.text.DateFormat.LONG)
        format.format(java.util.Date.from(instant))
    } catch (_: Exception) { iso }
    is UiText.Engine -> if (EngineKeys.isClosed(key)) EngineText.resolve(context, key) else context.getString(R.string.error_core_operation)
    // El resumen en lenguaje llano; las evidencias van aparte en VerificationCard.technical.
    is UiText.Verification -> VerificationCard.summary(this).joinToString("\n") { it.resolve(context) }
}

/** Origen de la verificación mostrada en la tarjeta de resultado. */
enum class VerificationOrigin { SIGN, VERIFY }

/** Sustituye un argumento de texto (p. ej. el nombre propuesto por el guardado). */
fun UiText.replacingArgument(old: String, new: String): UiText = when (this) {
    is UiText.Resource -> copy(arguments = arguments.map { arg ->
        when (arg) { old -> new; is UiText -> arg.replacingArgument(old, new); else -> arg }
    })
    is UiText.Plural -> copy(arguments = arguments.map { if (it == old) new else it })
    is UiText.Lines -> copy(lines = lines.map { it.replacingArgument(old, new) })
    else -> this
}

sealed interface OperationResult {
    data object Idle : OperationResult
    data class Success(val title: UiText, val detail: UiText? = null) : OperationResult
    /** Hecho decidido por la persona (descartar, cierre por seguridad): sin color de error. */
    data class Notice(val detail: UiText) : OperationResult
    data class Error(val detail: UiText) : OperationResult
}

data class MainUiState(
    val backend: CoreReadiness,
    val document: SelectedFile? = null,
    val originalDocument: SelectedFile? = null,
    val certificateFile: SelectedFile? = null,
    val certificate: CertificateSummary? = null,
    val busy: Boolean = false,
    /** Contraseña del fichero de certificado incorrecta: se marca en su campo. */
    val certificatePasswordError: UiText? = null,
    val awaitingSave: Boolean = false,
    val result: OperationResult = OperationResult.Idle,
    val verification: VerificationSummary? = null,
    /** De dónde viene [verification]: la firma recién creada o «Verificar firma». */
    val verificationOrigin: VerificationOrigin = VerificationOrigin.VERIFY,
    val verifiedDocumentName: String = "",
    val postSignVerificationFailed: Boolean = false,
    val signatureAction: String = "sign",
    val signatureProfile: String = "baseline",
    val tsaEnabled: Boolean = false,
    val tsaUrl: String = "",
    val coSignSuggested: Boolean = false,
    val detectedSignatureFormat: String = "",
    val awaitingReportSave: Boolean = false,
    val toolsAvailable: Boolean = false,
    val certificateExternal: Boolean = false,
    val pendingKind: PendingKind = PendingKind.SIGNATURE,
    val hashAlgorithm: String = "SHA-256",
    val hashFormat: String = "hex",
    val protectionContainer: String = "cms",
    val protectForMe: Boolean = true,
    val recipients: List<SelectedFile> = emptyList(),
    val batchDocuments: List<SelectedFile> = emptyList(),
    val signingFormats: List<String> = SignatureFormats.BASIC,
    val documentServices: Set<String> = emptySet(),
    val verifactuRecords: List<SelectedFile> = emptyList(),
    /** Fecha de captura ENI elegida en el calendario (medianoche UTC) o null para «ahora». */
    val eniCaptureDate: Long? = null,
    val platformServices: Set<String> = emptySet(),
    val settings: AppSettings = AppSettings(),
    /** Certificados abiertos en la sesión; [certificate] es el elegido para firmar. */
    val identities: List<SessionIdentity> = emptyList(),
    /** Certificados que el núcleo admite abiertos a la vez (1 con un AAR anterior). */
    val maxIdentities: Int = 1,
    /** Detalle de los certificados de la sesión con caducidad, en orden de apertura. */
    val certificateDetails: List<CertificateDetail> = emptyList(),
    val expiringSoonDays: Int = 30,
    val certificateFilter: String = "",
    val certificateKindFilter: String = "",
    val veriFactuReport: VeriFactuReport? = null,
    val reportKind: ReportKind = ReportKind.VERIFICATION,
    val veriFactuQr: VeriFactuQr? = null,
    val aeatResponse: String = "",
    val qrError: UiText? = null,
    val diagnostics: EngineDiagnostics? = null,
    val tsaProbe: TsaProbe? = null,
    val updateCheck: UpdateCheck? = null,
    /** Capacidades de la cuarta oleada declaradas por el núcleo. */
    val capabilities: Set<String> = emptySet(),
    val wave4: Wave4State = Wave4State(),
) {
    private val idle: Boolean get() = !busy && !awaitingSave && !awaitingReportSave
    private fun offers(service: String): Boolean = backend.available && service in documentServices
    val verifactuAvailable: Boolean get() = offers(DocumentServices.VERIFACTU)
    val eniDocumentAvailable: Boolean get() = offers(DocumentServices.ENI_DOCUMENT)
    val eniValidateAvailable: Boolean get() = offers(DocumentServices.ENI_VALIDATE)
    val csvLegendAvailable: Boolean get() = offers(DocumentServices.CSV_LEGEND)
    val canCheckVeriFactu: Boolean get() = verifactuAvailable && idle && verifactuRecords.isNotEmpty()
    private fun platform(service: String): Boolean = backend.available && service in platformServices
    val certificatePanelAvailable: Boolean get() = platform(PlatformServices.CERTIFICATE_DETAILS)
    val canCheckCertificateOnline: Boolean
        get() = platform(PlatformServices.CERTIFICATE_ONLINE) && idle && certificate != null
    val diagnosticsAvailable: Boolean get() = platform(PlatformServices.DIAGNOSTICS)
    val canProbeTsa: Boolean get() = platform(PlatformServices.TSA_PROBE) && idle && tsaUrl.isNotBlank()
    val qrReadAvailable: Boolean get() = platform(PlatformServices.VERIFACTU_QR_READ)
    val canReadQr: Boolean get() = qrReadAvailable && idle
    /** Leer el QR con la cámara o desde una imagen elegida. */
    val qrImageAvailable: Boolean get() = platform(PlatformServices.VERIFACTU_QR_IMAGE)
    val canReadQrImage: Boolean get() = qrImageAvailable && idle
    /** El cotejo exige una lectura previa válida: nunca se consulta una URL sin validar. */
    val canQueryAeat: Boolean get() = platform(PlatformServices.VERIFACTU_QR_QUERY) && idle && veriFactuQr != null
    val updateCheckAvailable: Boolean get() = platform(PlatformServices.UPDATE_CHECK)
    val canCheckUpdate: Boolean get() = updateCheckAvailable && idle
    val canExportVeriFactu: Boolean get() = veriFactuReport != null && idle
    /** Certificados que cumplen el filtro; buscar solo compensa a partir de [CERTIFICATE_FILTER_MIN]. */
    val filteredCertificates: List<CertificateDetail>
        get() = if (!showsCertificateFilter) certificateDetails
            else CertificateFilter.apply(certificateDetails, certificateFilter, certificateKindFilter)
    val showsCertificateFilter: Boolean get() = certificateDetails.size >= CERTIFICATE_FILTER_MIN
    /** Con más de un certificado abierto se elige en una lista con cuál firmar. */
    val showsIdentityList: Boolean get() = identities.size > 1
    val filteredIdentities: List<SessionIdentity>
        get() = if (!showsCertificateFilter) identities
            else SessionIdentities.filter(identities, certificateDetails, certificateFilter, certificateKindFilter)
    /** Se puede abrir otro certificado sin cerrar los que ya están abiertos. */
    val canKeepSeveralIdentities: Boolean get() = maxIdentities > 1
    val canChangeIdentity: Boolean get() = backend.available && idle
    val canCreateEni: Boolean get() = eniDocumentAvailable && idle && document != null
    val canValidateEni: Boolean get() = eniValidateAvailable && idle
    val canUseTools: Boolean get() = backend.available && toolsAvailable && idle
    val canHash: Boolean get() = canUseTools && document != null
    val canProtect: Boolean get() = canUseTools && document != null
    val canProtectAndSign: Boolean
        get() = canProtect && certificate != null && (!certificateExternal || externalProtectSignAvailable) &&
            protectionContainer != "cms-encrypted"
    /** Solo con un documento que puede estar protegido; si no, se explica junto al botón. */
    val canUnprotect: Boolean get() = canUseTools && document != null && unprotectSupported
    val unprotectSupported: Boolean get() = document?.let { ToolsPolicy.looksUnprotectable(it.displayName) } != false
    /** «Incluir mi certificado» con un certificado que no admite cifrado nunca puede funcionar. */
    val ownCertificateCannotEncrypt: Boolean
        get() = canProtectForMe && certificateDetails.firstOrNull { it.id == certificate?.id }?.canEncrypt == false
    val canSignBatch: Boolean
        get() = canUseTools && batchDocuments.isNotEmpty() && certificate != null &&
            (!certificateExternal || externalBatchAvailable)
    val usesTransientKey: Boolean get() = protectionContainer == "cms-encrypted"
    /** El certificado de FIRMA del DNIe no sirve para cifrar; solo el PKCS#12. */
    val canProtectForMe: Boolean get() = protectForMe && certificate != null && !certificateExternal
    val canExportReport: Boolean get() = verification?.reportJson?.isNotEmpty() == true && !busy && !awaitingSave && !awaitingReportSave
    val canImportCertificate: Boolean
        get() = backend.available && certificateFile != null && !busy && !awaitingSave && !awaitingReportSave
    val canSign: Boolean
        get() = backend.available && document != null && certificate != null && !busy && !awaitingSave && !awaitingReportSave
    val canVerify: Boolean
        get() = backend.available && document != null && !busy && !awaitingSave && !awaitingReportSave
    val canForgetCertificate: Boolean
        get() = backend.available && certificate != null && !busy && !awaitingSave && !awaitingReportSave
    val canRetryPendingOutput: Boolean get() = awaitingSave && !busy
    val canDiscardPendingOutput: Boolean get() = awaitingSave && !busy
    val canReplaceSelection: Boolean get() = !busy && !awaitingSave && !awaitingReportSave
    val canAcceptIncomingDocument: Boolean get() = !busy && !awaitingSave && !awaitingReportSave
}

/**
 * La tarjeta de resultado solo enseña la última operación: al empezar otra
 * (o al dar un resultado que no es una verificación) se borran el resultado
 * y la verificación anteriores, con sus botones de informe.
 */
fun MainUiState.clearedResult(result: OperationResult = OperationResult.Idle): MainUiState = copy(
    result = result,
    verification = null,
    verifiedDocumentName = "",
    postSignVerificationFailed = false,
)

/** Con menos certificados abiertos la lista se lee de un vistazo: no hay buscador. */
const val CERTIFICATE_FILTER_MIN = 5

/** Qué espera guardarse: una firma, un fichero de una herramienta o el lote. */
enum class PendingKind { SIGNATURE, TOOL, BATCH }

/** Qué informe se está exportando por SAF. */
enum class ReportKind { VERIFICATION, VERIFICATION_HTML, VERIFACTU }

/** Resultado de comprobar el cierre automático del certificado tras el segundo plano. */
enum class AutoClose { CLOSED, NOT_DUE, POSTPONED, NOT_APPLICABLE }

sealed interface UiEffect {
    data class SaveSignedDocument(val displayName: String, val mimeType: String) : UiEffect
    data object SaveVerificationReport : UiEffect
    data object ChooseBatchFolder : UiEffect
    data object SaveVeriFactuReport : UiEffect
    data object SaveVerificationHtml : UiEffect
    /** Abre la publicación oficial en el navegador; la URL ya está comprobada. */
    data class OpenRelease(val url: String) : UiEffect
}

fun Throwable.toUserText(): UiText {
    val resource = when (this) {
        is InvalidDocumentException -> when (problem) {
            es.dipgra.grxfirma.android.files.DocumentProblem.TOO_LARGE -> R.string.error_document_too_large
            es.dipgra.grxfirma.android.files.DocumentProblem.EMPTY -> R.string.error_document_empty
            es.dipgra.grxfirma.android.files.DocumentProblem.TOO_MANY_ENTRIES -> R.string.error_folder_too_many
            else -> R.string.error_document_access
        }
        is SecurityException -> R.string.error_document_access
        is CoreUnavailableException -> R.string.error_core_unavailable
        is CoreContractException -> when (message) {
            CORE_PKCS12_DECODE_MESSAGE -> R.string.error_pkcs12_password_or_legacy
            CORE_CERTIFICATE_NOT_CURRENT_MESSAGE -> R.string.error_certificate_not_current
            CORE_SIGNING_IDENTITY_UNSUPPORTED_MESSAGE -> R.string.error_signing_identity_unsupported
            CORE_HASH_FILE_INVALID_MESSAGE -> R.string.error_hash_file
            CORE_PROTECTION_KEY_INVALID_MESSAGE -> R.string.error_protect_key
            CORE_PROTECTION_RECIPIENT_MESSAGE -> R.string.error_protect_recipient
            CORE_UNPROTECT_FAILED_MESSAGE -> R.string.error_unprotect_failed
            CORE_PROTECT_SIGN_IDENTITY_MESSAGE -> R.string.error_protect_sign_identity
            CORE_FORMAT_REQUIRES_RSA_MESSAGE -> R.string.error_format_requires_rsa
            else -> {
                val key = message.orEmpty()
                EngineKeys.localResource(key)?.let { return UiText.Resource(it) }
                if (EngineKeys.isClosed(key)) return UiText.Engine(key)
                R.string.error_core_operation
            }
        }
        else -> R.string.error_operation_failed
    }
    return UiText.Resource(resource)
}

// Solo se localizan mensajes cerrados emitidos por la fachada. Cualquier
// detalle inesperado del núcleo conserva el error genérico y no llega a la UI.
private const val CORE_PKCS12_DECODE_MESSAGE =
    "No se pudo abrir el PKCS#12. Compruebe la contraseña; si usa un formato antiguo, reexpórtelo como PKCS#12 moderno (AES y SHA-256)."
private const val CORE_CERTIFICATE_NOT_CURRENT_MESSAGE =
    "El certificado no está vigente. Use un certificado vigente; si ha caducado, renuévelo."
private const val CORE_SIGNING_IDENTITY_UNSUPPORTED_MESSAGE =
    "El certificado o su clave no son aptos para firmar en Android. Use un certificado de firma con RSA de al menos 2048 bits o ECDSA de al menos 256 bits."

private const val CORE_HASH_FILE_INVALID_MESSAGE =
    "El fichero de huella no es válido o usa un algoritmo no admitido."
private const val CORE_PROTECTION_KEY_INVALID_MESSAGE =
    "La clave de EncryptedData debe ser AES-256 en Base64 canónico (44 caracteres)."
private const val CORE_PROTECTION_RECIPIENT_MESSAGE =
    "El certificado del destinatario no permite cifrar. Use un certificado público RSA vigente de al menos 2048 bits con cifrado de clave."
private const val CORE_UNPROTECT_FAILED_MESSAGE =
    "No se pudo desproteger el fichero. Compruebe que va dirigido a su certificado o que la clave es correcta."
private const val CORE_FORMAT_REQUIRES_RSA_MESSAGE =
    "El formato elegido solo admite certificados con clave RSA."
private const val CORE_PROTECT_SIGN_IDENTITY_MESSAGE =
    "Para proteger y firmar hace falta un certificado PKCS#12 importado en la sesión."

fun VerificationSummary.toUiText() = UiText.Verification(
    valid, reason, format, signers.size, integrityStatus, certificateStatus, trustStatus,
    revocationMode, warnings.size, errors.size, coverage, signers, signerSummaries, details, warnings, errors,
)

/** Visibilidad de botones de la tercera oleada (el AAR los declara o no). */
object PlatformServicesVisibility {
    fun online(state: MainUiState): Boolean =
        state.backend.available && es.dipgra.grxfirma.android.core.PlatformServices.CERTIFICATE_ONLINE in state.platformServices
    fun aeat(state: MainUiState): Boolean =
        state.backend.available && es.dipgra.grxfirma.android.core.PlatformServices.VERIFACTU_QR_QUERY in state.platformServices
    fun tsa(state: MainUiState): Boolean =
        state.backend.available && es.dipgra.grxfirma.android.core.PlatformServices.TSA_PROBE in state.platformServices
}
