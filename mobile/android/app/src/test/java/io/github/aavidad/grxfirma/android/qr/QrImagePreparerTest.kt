// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.qr

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class QrImagePreparerTest {
    @Test fun `recognises PNG and JPEG by their signature`() {
        assertTrue(QrImagePreparer.isPngOrJpeg(byteArrayOf(0x89.toByte(), 'P'.code.toByte(), 'N'.code.toByte(), 'G'.code.toByte(), 13, 10, 26, 10)))
        assertTrue(QrImagePreparer.isPngOrJpeg(byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 0xFF.toByte())))
        assertFalse(QrImagePreparer.isPngOrJpeg("RIFF0000WEBP".encodeToByteArray()))
        assertFalse(QrImagePreparer.isPngOrJpeg(ByteArray(0)))
    }

    @Test fun `only converts what the core would reject`() {
        assertFalse(QrImagePreparer.needsConversion(4000, 3000, 5_000_000, pngOrJpeg = true))
        assertTrue(QrImagePreparer.needsConversion(8160, 6120, 9_000_000, pngOrJpeg = true)) // 50 Mpx
        assertTrue(QrImagePreparer.needsConversion(13000, 100, 1_000, pngOrJpeg = true))
        assertTrue(QrImagePreparer.needsConversion(1000, 1000, QrImagePreparer.MAX_CORE_BYTES + 1, pngOrJpeg = true))
        assertTrue(QrImagePreparer.needsConversion(1000, 1000, 100_000, pngOrJpeg = false))
    }

    @Test fun `sample size keeps the longest side at the target or less`() {
        assertEquals(1, QrImagePreparer.sampleSize(4000, 3000))
        assertEquals(2, QrImagePreparer.sampleSize(12000, 9000))
        assertEquals(4, QrImagePreparer.sampleSize(20000, 100))
        assertEquals(2, QrImagePreparer.sampleSize(8160, 6120))
        assertTrue(QrImagePreparer.sampleSize(Int.MAX_VALUE, 1) <= 256)
    }
}
