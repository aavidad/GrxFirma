// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import android.view.KeyEvent
import io.github.aavidad.grxfirma.android.R
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SealAdjustmentTest {
    @Test fun `keyboard moves resizes and rotates the seal`() {
        assertEquals(SealAdjustment.LEFT, SealAdjustment.forKey(KeyEvent.KEYCODE_DPAD_LEFT))
        assertEquals(SealAdjustment.RIGHT, SealAdjustment.forKey(KeyEvent.KEYCODE_DPAD_RIGHT))
        assertEquals(SealAdjustment.UP, SealAdjustment.forKey(KeyEvent.KEYCODE_DPAD_UP))
        assertEquals(SealAdjustment.DOWN, SealAdjustment.forKey(KeyEvent.KEYCODE_DPAD_DOWN))
        assertEquals(SealAdjustment.LARGER, SealAdjustment.forKey(KeyEvent.KEYCODE_PLUS))
        assertEquals(SealAdjustment.LARGER, SealAdjustment.forKey(KeyEvent.KEYCODE_NUMPAD_ADD))
        assertEquals(SealAdjustment.SMALLER, SealAdjustment.forKey(KeyEvent.KEYCODE_MINUS))
        assertEquals(SealAdjustment.ROTATE_LEFT, SealAdjustment.forKey(KeyEvent.KEYCODE_COMMA))
        assertEquals(SealAdjustment.ROTATE_RIGHT, SealAdjustment.forKey(KeyEvent.KEYCODE_PERIOD))
        // Tabulador e Intro siguen moviendo el foco y activando controles.
        assertNull(SealAdjustment.forKey(KeyEvent.KEYCODE_TAB))
        assertNull(SealAdjustment.forKey(KeyEvent.KEYCODE_ENTER))
    }

    @Test fun `buttons keys and TalkBack actions reuse the same texts`() {
        assertEquals(
            listOf(R.string.seal_move_left, R.string.seal_move_right, R.string.seal_move_up, R.string.seal_move_down,
                R.string.seal_smaller_desc, R.string.seal_larger_desc, R.string.seal_rotate_left_desc,
                R.string.seal_rotate_right_desc),
            SealAdjustment.entries.map { it.label },
        )
        assertEquals(-15, SealAdjustment.ROTATE_LEFT.angle)
        assertEquals(0.02f, SealAdjustment.UP.dy)
    }

    @Test fun `state names page position size and rotation in whole percentages`() {
        val settings = SealSettings(enabled = true, rect = SealRect(x = 0.604f, y = 0.055f, w = 0.32f, h = 0.11f),
            rotation = 15)
        assertEquals(SealState(2, 5, 60, 6, 32, 11, 15), SealAdjustment.state(settings, 2, 5))
        // Sin páginas cargadas todavía no se dice «página 1 de 0».
        assertEquals(1, SealAdjustment.state(settings, 1, 0).pages)
    }
}
