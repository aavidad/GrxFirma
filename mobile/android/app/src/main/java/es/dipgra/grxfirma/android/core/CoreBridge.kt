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
}

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
