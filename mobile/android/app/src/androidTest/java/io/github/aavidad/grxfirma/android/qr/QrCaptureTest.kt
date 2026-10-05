// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.qr

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Color
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.io.ByteArrayOutputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class QrCaptureTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    @Test fun photoFileIsPrivateSharedByContentUriAndWiped() {
        val uri = QrCapture.prepare(context)
        assertEquals("content", uri.scheme)
        assertEquals(QrCapture.authority(context), uri.authority)
        val file = QrCapture.file(context)
        assertTrue(file.isFile)
        assertTrue(file.canonicalPath.startsWith(context.cacheDir.canonicalPath))
        context.contentResolver.openOutputStream(uri)!!.use { it.write(ByteArray(1024) { 7 }) }
        assertEquals(1024L, file.length())
        QrCapture.clear(context)
        assertFalse(file.exists())
    }

    @Test fun preparingAgainRemovesAnOldPhoto() {
        QrCapture.prepare(context)
        QrCapture.file(context).writeBytes(ByteArray(10) { 1 })
        QrCapture.prepare(context)
        assertEquals(0L, QrCapture.file(context).length())
        QrCapture.clear(context)
    }

    @Test fun webpIsConvertedToJpegAndJpegIsKept() {
        val bitmap = Bitmap.createBitmap(200, 200, Bitmap.Config.ARGB_8888).apply { eraseColor(Color.WHITE) }
        val webp = ByteArrayOutputStream().use { out ->
            @Suppress("DEPRECATION")
            bitmap.compress(Bitmap.CompressFormat.WEBP, 90, out)
            out.toByteArray()
        }
        val jpeg = ByteArrayOutputStream().use { out ->
            bitmap.compress(Bitmap.CompressFormat.JPEG, 90, out)
            out.toByteArray()
        }
        bitmap.recycle()
        val converted = QrImagePreparer.prepare(webp)
        assertTrue(QrImagePreparer.isPngOrJpeg(converted))
        assertTrue(webp.all { it == 0.toByte() }) // el original convertido se borra
        assertSame(jpeg, QrImagePreparer.prepare(jpeg))
        val unknown = byteArrayOf(1, 2, 3)
        assertSame(unknown, QrImagePreparer.prepare(unknown))
    }
}
