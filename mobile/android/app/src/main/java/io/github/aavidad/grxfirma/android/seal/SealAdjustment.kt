// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import android.view.KeyEvent
import androidx.annotation.StringRes
import io.github.aavidad.grxfirma.android.R
import kotlin.math.roundToInt

/**
 * Un paso de ajuste del sello. Los botones de debajo de la página, el teclado
 * y las acciones de TalkBack usan los mismos pasos y los mismos textos.
 */
enum class SealAdjustment(
    @param:StringRes val label: Int,
    val dx: Float = 0f,
    val dy: Float = 0f,
    val dw: Float = 0f,
    val dh: Float = 0f,
    val angle: Int = 0,
) {
    LEFT(R.string.seal_move_left, dx = -0.02f),
    RIGHT(R.string.seal_move_right, dx = 0.02f),
    UP(R.string.seal_move_up, dy = 0.02f),
    DOWN(R.string.seal_move_down, dy = -0.02f),
    SMALLER(R.string.seal_smaller_desc, dw = -0.02f, dh = -0.01f),
    LARGER(R.string.seal_larger_desc, dw = 0.02f, dh = 0.01f),
    ROTATE_LEFT(R.string.seal_rotate_left_desc, angle = -15),
    ROTATE_RIGHT(R.string.seal_rotate_right_desc, angle = 15);

    companion object {
        /** Flechas para mover, + y - para el tamaño, coma o [ y punto o ] para girar. */
        fun forKey(keyCode: Int): SealAdjustment? = when (keyCode) {
            KeyEvent.KEYCODE_DPAD_LEFT -> LEFT
            KeyEvent.KEYCODE_DPAD_RIGHT -> RIGHT
            KeyEvent.KEYCODE_DPAD_UP -> UP
            KeyEvent.KEYCODE_DPAD_DOWN -> DOWN
            KeyEvent.KEYCODE_MINUS, KeyEvent.KEYCODE_NUMPAD_SUBTRACT -> SMALLER
            KeyEvent.KEYCODE_PLUS, KeyEvent.KEYCODE_NUMPAD_ADD, KeyEvent.KEYCODE_EQUALS -> LARGER
            KeyEvent.KEYCODE_COMMA, KeyEvent.KEYCODE_LEFT_BRACKET -> ROTATE_LEFT
            KeyEvent.KEYCODE_PERIOD, KeyEvent.KEYCODE_RIGHT_BRACKET -> ROTATE_RIGHT
            else -> null
        }

        /** Lo que dice `seal_canvas_state`, con las medidas en % entero de la página. */
        fun state(settings: SealSettings, page: Int, pages: Int): SealState {
            val rect = settings.rect
            return SealState(page, pages.coerceAtLeast(page), percent(rect.x), percent(rect.y),
                percent(rect.w), percent(rect.h), settings.rotation)
        }

        private fun percent(value: Float): Int = (value * 100f).roundToInt().coerceIn(0, 100)
    }
}

/**
 * Página, total, distancia a los bordes izquierdo e inferior, ancho y alto
 * (en % de la página) y giro en grados.
 */
data class SealState(
    val page: Int,
    val pages: Int,
    val left: Int,
    val bottom: Int,
    val width: Int,
    val height: Int,
    val rotation: Int,
)
