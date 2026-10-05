// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class SealGeometryTest {
    @Test
    fun `screen top left converts to PDF bottom left`() {
        val rect = SealGeometry.fromScreen(100f, 200f, 200f, 80f, 500f, 1000f)
        assertEquals(0.2f, rect.x, 0.0001f)
        assertEquals(0.72f, rect.y, 0.0001f)
        assertEquals(0.4f, rect.w, 0.0001f)
        assertEquals(0.08f, rect.h, 0.0001f)
    }

    @Test
    fun `rotated seal keeps original dimensions and fits a portrait page`() {
        val rect = SealRect(0.7f, 0.8f, 0.25f, 0.10f)
        val fitted = SealGeometry.fromScreenWithRotation(420f, 100f, 150f, 100f, 600f, 1000f, 30)
        val (boundW, boundH) = SealGeometry.rotatedBounds(fitted, 30, 0.6f)
        assertEquals(rect.w, fitted.w)
        assertEquals(rect.h, fitted.h)
        assertTrue(fitted.x + fitted.w / 2f + boundW / 2f <= 1.0001f)
        assertTrue(fitted.y + fitted.h / 2f + boundH / 2f <= 1.0001f)
    }

    @Test
    fun `rotation snaps near right angles and rejects oversized bounds`() {
        assertEquals(90, SealGeometry.snap(86))
        assertEquals(30, SealGeometry.snap(30))
        assertEquals(0, SealGeometry.snap(357))
        assertThrows(IllegalArgumentException::class.java) {
            SealGeometry.fit(SealRect(0f, 0f, 0.9f, 0.9f), 45, 1f)
        }
    }

    @Test
    fun `QR accepts only HTTPS and adds scheme`() {
        assertEquals("https://example.org/check", normalizedHttps("example.org/check"))
        assertThrows(IllegalArgumentException::class.java) { normalizedHttps("http://example.org") }
        assertThrows(IllegalArgumentException::class.java) { normalizedHttps("https://user:pass@example.org") }
    }
}
