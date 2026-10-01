// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android

import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.test.core.app.ApplicationProvider
import es.dipgra.grxfirma.android.core.ReflectiveGomobileBridge
import es.dipgra.grxfirma.android.core.CoreBridge
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.seal.SealRect
import es.dipgra.grxfirma.android.seal.SealSettings
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

class CoreVisibleSealSigningTest {
    @Test
    fun signsValidPdfWithThirtyDegreeSealAtHalfOpacity() {
        assumeTrue(BuildConfig.CORE_MODE == "production")
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val input = File(context.filesDir, "qa-input")
        val pdf = File(input, CoreProductionSigningTest.PDF_FILENAME)
        val p12 = File(input, CoreProductionSigningTest.P12_FILENAME)
        assumeTrue(pdf.isFile && p12.isFile)
        val password = CoreProductionSigningTest.TEST_PASSWORD.toCharArray()
        val certificateBytes = p12.readBytes()
        val originalBytes = pdf.readBytes()
        var signedBytes: ByteArray? = null
        var session: CoreBridge? = null
        try {
            val core = ReflectiveGomobileBridge.create(context)
            session = core
            assertTrue(core.readiness.available)
            val certificate = core.importCertificate(certificateBytes, password)
            val settings = SealSettings(enabled = true, rect = SealRect(0.25f, 0.10f, 0.35f, 0.12f),
                rotation = 30, opacity = 50)
            val options = settings.options(1, 595, 842)
            assertEquals("50", options["visibleSealLogoOpacityPercent"])
            assertTrue(options["visibleSealPlacements"].orEmpty().contains("\"rotation\":30"))
            val preview = core.sealPreview(certificate.id, options)
            assertTrue(preview.size > 8)
            preview.fill(0)
            val output = core.sign(LoadedFile(pdf.name, "application/pdf", originalBytes), "pades", certificate.id, options)
            signedBytes = output.bytes
            assertEquals("PAdES", output.format)
            val verification = core.verify(LoadedFile(output.displayName, output.mimeType, output.bytes))
            assertTrue("El núcleo rechazó la firma visible: ${verification.errors}", verification.valid)
            val signedFile = File(context.filesDir, "qa-output/firma-android-qa-sello.pdf")
            assertTrue(signedFile.parentFile?.isDirectory == true || signedFile.parentFile?.mkdirs() == true)
            signedFile.writeBytes(output.bytes)
            ParcelFileDescriptor.open(signedFile, ParcelFileDescriptor.MODE_READ_ONLY).use { descriptor ->
                PdfRenderer(descriptor).use { renderer ->
                    assertTrue(renderer.pageCount >= 1)
                    renderer.openPage(0).use { page ->
                        assertTrue(page.width > 0 && page.height > 0)
                    }
                }
            }
        } finally {
            try { session?.clearSession() } finally {
                certificateBytes.fill(0)
                originalBytes.fill(0)
                signedBytes?.fill(0)
                password.fill('\u0000')
            }
        }
    }
}
