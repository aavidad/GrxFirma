// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import androidx.test.core.app.ApplicationProvider
import io.github.aavidad.grxfirma.android.core.ReflectiveGomobileBridge
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

class CoreProductionSigningTest {
    @Test
    fun productionCoreSignsAndVerifiesPadesCadesAndXadesWithSyntheticCertificate() {
        assumeTrue("Solo se ejecuta contra el AAR productivo.", BuildConfig.CORE_MODE == "production")

        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val p12File = File(context.filesDir, "$INPUT_DIRECTORY/$P12_FILENAME")
        val pdfFile = File(context.filesDir, "$INPUT_DIRECTORY/$PDF_FILENAME")
        assumeTrue(
            "Use scripts/mobile/android/test-production-signing.sh para preparar las fixtures efímeras.",
            p12File.isFile && pdfFile.isFile,
        )

        val p12 = p12File.readBytes()
        val password = TEST_PASSWORD.toCharArray()
        val cases = mutableListOf<SigningCase>()
        try {
            val core = ReflectiveGomobileBridge.create(context)
            assertTrue("El AAR productivo debe exponer el núcleo real.", core.readiness.available)

            val certificate = core.importCertificate(p12, password)
            assertTrue(certificate.id.isNotBlank())
            assertTrue(
                "Se esperaba el certificado sintético de la campaña Android.",
                certificate.subject.contains(TEST_SUBJECT),
            )

            cases += signCase(
                core,
                LoadedFile(PDF_FILENAME, "application/pdf", pdfFile.readBytes()),
                "pades",
                certificate.id,
            )
            cases += signCase(
                core,
                LoadedFile("android-qa.txt", "text/plain", ORIGINAL_TEXT.encodeToByteArray()),
                "cades",
                certificate.id,
                detached = true,
            )
            cases += signCase(
                core,
                LoadedFile(
                    "android-qa.xml",
                    "application/xml",
                    "<qa><product>GrxFirma</product><platform>Android</platform></qa>".encodeToByteArray(),
                ),
                "xades",
                certificate.id,
            )

            val outputDirectory = File(context.filesDir, OUTPUT_DIRECTORY)
            assertTrue(outputDirectory.isDirectory || outputDirectory.mkdirs())
            cases.forEach { signingCase ->
                val output = signingCase.output
                val expectedFormat = when (output.mimeType) {
                    "application/pdf" -> "PAdES"
                    "application/pkcs7-signature" -> "CAdES"
                    else -> "XAdES"
                }
                assertEquals(expectedFormat, output.format)
                assertTrue(output.bytes.isNotEmpty())
                File(outputDirectory, evidenceName(output)).writeBytes(output.bytes)

                val verification = core.verify(
                    LoadedFile(output.displayName, output.mimeType, output.bytes),
                    signingCase.original.takeIf { signingCase.detached },
                )
                assertTrue(
                    "${output.format} no superó la verificación: ${verification.errors}",
                    verification.valid,
                )
                assertEquals(output.format, verification.format)
                assertTrue(verification.signers.isNotEmpty())
                assertTrue(verification.errors.isEmpty())
            }
        } finally {
            password.fill('\u0000')
            p12.fill(0)
            cases.forEach {
                it.original.bytes.fill(0)
                it.output.bytes.fill(0)
            }
        }
    }

    private fun signCase(
        core: io.github.aavidad.grxfirma.android.core.CoreBridge,
        original: LoadedFile,
        format: String,
        certificateId: String,
        detached: Boolean = false,
    ): SigningCase = try {
        SigningCase(
            original = original,
            output = core.sign(original, format, certificateId),
            detached = detached,
        )
    } catch (error: Exception) {
        original.bytes.fill(0)
        throw error
    }

    private fun evidenceName(output: SignedOutput): String = when (output.format) {
        "PAdES" -> "firma-android-qa-pades.pdf"
        "CAdES" -> "firma-android-qa-cades.p7s"
        "XAdES" -> "firma-android-qa-xades.xml"
        else -> error("Formato de evidencia no esperado: ${output.format}")
    }

    companion object {
        const val INPUT_DIRECTORY = "qa-input"
        const val OUTPUT_DIRECTORY = "qa-output"
        const val P12_FILENAME = "grxfirma-android-qa.p12"
        const val PDF_FILENAME = "grxfirma-android-qa.pdf"
        const val TEST_PASSWORD = "valor-prueba"
        const val TEST_SUBJECT = "GrxFirma Android QA synthetic"
        const val ORIGINAL_TEXT = "GrxFirma Android QA\n"
    }

    private data class SigningCase(
        val original: LoadedFile,
        val output: SignedOutput,
        val detached: Boolean,
    )
}
