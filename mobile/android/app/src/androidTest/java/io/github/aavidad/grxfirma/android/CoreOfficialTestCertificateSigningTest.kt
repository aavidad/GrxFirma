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
import java.nio.ByteBuffer
import java.nio.CharBuffer
import java.nio.charset.CodingErrorAction
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

class CoreOfficialTestCertificateSigningTest {
    @Test
    fun officialFnmtTestCertificateProducesVerifiableSignatures() {
        assumeTrue("Solo se ejecuta contra el AAR productivo.", BuildConfig.CORE_MODE == "production")

        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val p12File = File(context.filesDir, "$INPUT_DIRECTORY/$P12_FILENAME")
        val pdfFile = File(context.filesDir, "$INPUT_DIRECTORY/$PDF_FILENAME")
        val passwordFile = File(context.filesDir, "$INPUT_DIRECTORY/$PASSWORD_FILENAME")
        assumeTrue(
            "Use scripts/mobile/android/test-official-certificate-signing.sh para preparar la prueba.",
            p12File.isFile && pdfFile.isFile && passwordFile.isFile,
        )
        assertTrue(
            "El secreto QA debe tener un tamaño acotado.",
            passwordFile.length() in 1..MAX_PASSWORD_BYTES.toLong(),
        )

        val p12 = p12File.readBytes()
        val passwordBytes = passwordFile.readBytes()
        val password = decodePassword(passwordBytes)
        val cases = mutableListOf<SigningCase>()
        try {
            val core = ReflectiveGomobileBridge.create(context)
            assertTrue("El AAR productivo debe exponer el núcleo real.", core.readiness.available)

            val certificate = core.importCertificate(p12, password)
            assertTrue(
                "No se cargó el certificado oficial de pruebas esperado.",
                certificate.subject.contains(TEST_SUBJECT_FRAGMENT),
            )

            cases += signCase(
                core,
                LoadedFile(PDF_FILENAME, "application/pdf", pdfFile.readBytes()),
                "pades",
                certificate.id,
            )
            cases += signCase(
                core,
                LoadedFile("fnmt-android-qa.txt", "text/plain", ORIGINAL_TEXT.encodeToByteArray()),
                "cades",
                certificate.id,
                detached = true,
            )
            cases += signCase(
                core,
                LoadedFile(
                    "fnmt-android-qa.xml",
                    "application/xml",
                    "<qa><certificate>FNMT 99999999R</certificate><platform>Android</platform></qa>"
                        .encodeToByteArray(),
                ),
                "xades",
                certificate.id,
            )

            val outputDirectory = File(context.filesDir, OUTPUT_DIRECTORY)
            assertTrue(outputDirectory.isDirectory || outputDirectory.mkdirs())
            val verificationReport = buildString {
                appendLine("subject=${certificate.subject}")
                appendLine("note=certificado oficial QA; comprobar integridad, vigencia y confianza por separado")
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
                    assertEquals(output.format, verification.format)
                    appendLine(
                        listOf(
                            "format=${verification.format}",
                            "valid=${verification.valid}",
                            "reason=${verification.reason}",
                            "signers=${verification.signers.joinToString("|")}",
                            "warnings=${verification.warnings.joinToString("|")}",
                            "errors=${verification.errors.joinToString("|")}",
                        ).joinToString("\t"),
                    )
                }
            }
            File(outputDirectory, REPORT_FILENAME).writeText(verificationReport)
        } finally {
            password.fill('\u0000')
            passwordBytes.fill(0)
            p12.fill(0)
            cases.forEach {
                it.original.bytes.fill(0)
                it.output.bytes.fill(0)
            }
            passwordFile.delete()
            p12File.delete()
        }
    }

    private fun decodePassword(encoded: ByteArray): CharArray {
        val scratch = CharArray(encoded.size)
        val target = CharBuffer.wrap(scratch)
        try {
            val decoder = Charsets.UTF_8.newDecoder()
                .onMalformedInput(CodingErrorAction.REPORT)
                .onUnmappableCharacter(CodingErrorAction.REPORT)
            val decoded = decoder.decode(ByteBuffer.wrap(encoded), target, true)
            assertTrue("El secreto QA no es UTF-8 válido.", !decoded.isError)
            val flushed = decoder.flush(target)
            assertTrue("No se pudo completar la lectura del secreto QA.", !flushed.isError)
            assertTrue("El secreto QA está vacío.", target.position() > 0)
            return scratch.copyOf(target.position())
        } finally {
            scratch.fill('\u0000')
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
        "PAdES" -> "firma-fnmt-oficial-pades.pdf"
        "CAdES" -> "firma-fnmt-oficial-cades.p7s"
        "XAdES" -> "firma-fnmt-oficial-xades.xml"
        else -> error("Formato de evidencia no esperado: ${output.format}")
    }

    companion object {
        const val INPUT_DIRECTORY = "qa-input"
        const val OUTPUT_DIRECTORY = "qa-output-official"
        const val P12_FILENAME = "grxfirma-fnmt-oficial.p12"
        const val PDF_FILENAME = "grxfirma-fnmt-oficial.pdf"
        const val PASSWORD_FILENAME = "grxfirma-fnmt-oficial.password"
        const val REPORT_FILENAME = "verificacion-fnmt-oficial.txt"
        const val MAX_PASSWORD_BYTES = 1024
        const val TEST_SUBJECT_FRAGMENT = "99999999R"
        const val ORIGINAL_TEXT = "GrxFirma Android official FNMT QA\n"
    }

    private data class SigningCase(
        val original: LoadedFile,
        val output: SignedOutput,
        val detached: Boolean,
    )
}
