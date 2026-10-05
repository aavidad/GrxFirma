// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.seal

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Rect
import android.graphics.RectF
import androidx.core.graphics.withRotation
import android.view.MotionEvent
import android.view.View
import es.dipgra.grxfirma.android.R
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.hypot
import kotlin.math.sin

class SealCanvasView(context: Context) : View(context) {
    var pageBitmap: Bitmap? = null
        set(value) { field = value; requestLayout(); invalidate() }
    var sealBitmap: Bitmap? = null
        set(value) { field = value; invalidate() }
    var settings: SealSettings = SealSettings(enabled = true)
        set(value) { field = value; contentDescription = context.getString(R.string.seal_canvas_description, value.rotation); invalidate() }
    var onEdited: ((SealSettings) -> Unit)? = null
    /** En «varias páginas», false indica que esta página aún no lleva sello. */
    var sealOnPage: Boolean = true
        set(value) { field = value; invalidate() }

    private val border = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = 0xFF005A46.toInt(); style = Paint.Style.STROKE; strokeWidth = 3f * resources.displayMetrics.density }
    private val handle = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = 0xFF005A46.toInt(); style = Paint.Style.FILL }
    private val rotateGlyph = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = 0xFFFFFFFF.toInt()
        textAlign = Paint.Align.CENTER
        textSize = 18f * resources.displayMetrics.scaledDensity
    }
    private val placeholder = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = 0x55005A46; style = Paint.Style.FILL }
    private val handleRadius = 24f * resources.displayMetrics.density
    private var gesture = Gesture.NONE
    private var lastX = 0f
    private var lastY = 0f
    private var lastAngle = 0.0

    private enum class Gesture { NONE, MOVE, RESIZE, ROTATE, TWO_FINGERS }

    init {
        isFocusable = true
        importantForAccessibility = IMPORTANT_FOR_ACCESSIBILITY_YES
        contentDescription = context.getString(R.string.seal_canvas_description, 0)
    }

    /**
     * Alto máximo de la página en píxeles (0 = sin límite). Con él asoman bajo
     * la página los botones de ajuste que anuncia el texto de ayuda.
     */
    var maxPageHeight: Int = 0
        set(value) { field = value; requestLayout() }

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        var w = MeasureSpec.getSize(widthMeasureSpec)
        val bitmap = pageBitmap
        val ratio = if (bitmap == null || bitmap.width <= 0) 4f / 3f else bitmap.height.toFloat() / bitmap.width
        var h = (w * ratio).toInt()
        if (maxPageHeight in 1 until h) {
            // Se reduce la página entera, sin recortarla ni deformarla.
            h = maxPageHeight
            w = (h / ratio).toInt()
        }
        setMeasuredDimension(w.coerceAtLeast(1), h.coerceAtLeast(1))
    }

    // Rectángulos reutilizados: onDraw no debe reservar memoria en cada fotograma.
    private val pageDst = Rect()
    private val sealDst = RectF()

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        pageBitmap?.let {
            pageDst.set(0, 0, width, height)
            canvas.drawBitmap(it, null, pageDst, null)
        }
        val r = settings.rect
        val left = r.x * width
        val top = (1f - r.y - r.h) * height
        val right = left + r.w * width
        val bottom = top + r.h * height
        canvas.withRotation(settings.rotation.toFloat(), (left + right) / 2f, (top + bottom) / 2f) {
            val seal = sealBitmap?.takeIf { sealOnPage }
            if (seal != null) {
                sealDst.set(left, top, right, bottom)
                drawBitmap(seal, null, sealDst, null)
            } else {
                drawRect(left, top, right, bottom, placeholder)
            }
            drawRect(left, top, right, bottom, border)
            drawCircle(right, bottom, handleRadius / 2f, handle)
            drawCircle((left + right) / 2f, top - handleRadius, handleRadius / 2f, handle)
            drawText("↻", (left + right) / 2f, top - handleRadius + rotateGlyph.textSize / 3f, rotateGlyph)
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (width == 0 || height == 0) return false
        val r = settings.rect
        val left = r.x * width
        val top = (1f - r.y - r.h) * height
        val right = left + r.w * width
        val bottom = top + r.h * height
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                lastX = event.x; lastY = event.y
                val theta = Math.toRadians(settings.rotation.toDouble())
                val cx = (left + right) / 2f; val cy = (top + bottom) / 2f
                val localX = (cx + (event.x - cx) * cos(theta) + (event.y - cy) * sin(theta)).toFloat()
                val localY = (cy - (event.x - cx) * sin(theta) + (event.y - cy) * cos(theta)).toFloat()
                gesture = when {
                    hypot(localX - right, localY - bottom) <= handleRadius -> Gesture.RESIZE
                    hypot(localX - cx, localY - (top - handleRadius)) <= handleRadius -> Gesture.ROTATE
                    localX in left..right && localY in top..bottom -> Gesture.MOVE
                    else -> Gesture.NONE
                }
                if (gesture != Gesture.NONE) parent.requestDisallowInterceptTouchEvent(true)
                return gesture != Gesture.NONE
            }
            MotionEvent.ACTION_POINTER_DOWN -> if (event.pointerCount == 2) {
                gesture = Gesture.TWO_FINGERS
                lastAngle = fingerAngle(event)
                parent.requestDisallowInterceptTouchEvent(true)
                return true
            }
            MotionEvent.ACTION_MOVE -> {
                val next = when (gesture) {
                    Gesture.MOVE -> settings.copy(rect = r.copy(
                        x = r.x + (event.x - lastX) / width,
                        y = r.y - (event.y - lastY) / height,
                    ))
                    Gesture.RESIZE -> {
                        val theta = Math.toRadians(settings.rotation.toDouble())
                        val dx = event.x - lastX; val dy = event.y - lastY
                        val localDx = (dx * cos(theta) + dy * sin(theta)).toFloat()
                        val localDy = (-dx * sin(theta) + dy * cos(theta)).toFloat()
                        val nextH = (r.h + localDy / height).coerceIn(0.04f, 0.90f)
                        settings.copy(rect = r.copy(
                            w = (r.w + localDx / width).coerceIn(0.08f, 0.90f),
                            h = nextH,
                            y = r.y - (nextH - r.h),
                        ))
                    }
                    Gesture.ROTATE -> {
                        val cx = (left + right) / 2f; val cy = (top + bottom) / 2f
                        val angle = Math.toDegrees(atan2((event.y - cy).toDouble(), (event.x - cx).toDouble())).toInt() + 90
                        settings.copy(rotation = SealGeometry.snap(angle))
                    }
                    Gesture.TWO_FINGERS -> if (event.pointerCount >= 2) {
                        val angle = fingerAngle(event)
                        val delta = Math.toDegrees(angle - lastAngle).toInt()
                        lastAngle = angle
                        settings.copy(rotation = SealGeometry.snap(settings.rotation + delta))
                    } else settings
                    Gesture.NONE -> settings
                }
                try {
                    val fitted = next.copy(rect = SealGeometry.fit(next.rect, next.rotation, width.toFloat() / height))
                    if (fitted.rect.valid()) {
                        settings = fitted
                        onEdited?.invoke(fitted)
                    }
                } catch (_: IllegalArgumentException) { /* Mantener la última geometría válida. */ }
                lastX = event.x; lastY = event.y
                return true
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                if (gesture != Gesture.NONE) performClick()
                gesture = Gesture.NONE
                parent.requestDisallowInterceptTouchEvent(false)
                return true
            }
        }
        return true
    }

    override fun performClick(): Boolean {
        super.performClick()
        return true
    }

    private fun fingerAngle(event: MotionEvent): Double = atan2(
        (event.getY(1) - event.getY(0)).toDouble(),
        (event.getX(1) - event.getX(0)).toDouble(),
    )
}
