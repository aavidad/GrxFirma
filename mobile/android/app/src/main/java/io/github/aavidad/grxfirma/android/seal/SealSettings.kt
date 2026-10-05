// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import android.content.Context
import androidx.core.content.edit
import org.json.JSONArray
import org.json.JSONObject
import java.net.URI
import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.sin

data class SealRect(val x: Float = 0.60f, val y: Float = 0.06f, val w: Float = 0.32f, val h: Float = 0.11f) {
    fun valid(): Boolean = x >= 0f && y >= 0f && w > 0f && h > 0f && x + w <= 1f && y + h <= 1f &&
        listOf(x, y, w, h).all(Float::isFinite)
}

/** Posición propia del sello en una página concreta. */
data class SealPlacement(val rect: SealRect, val rotation: Int) {
    fun valid(): Boolean = rect.valid() && rotation in 0..359
}

data class SealSettings(
    val enabled: Boolean = false,
    val rect: SealRect = SealRect(),
    val rotation: Int = 0,
    val opacity: Int = 100,
    val page: Int = 1,
    val allPages: Boolean = false,
    val qrEnabled: Boolean = false,
    val qrAddress: String = "",
    val logo: String = "none",
    val keepText: Boolean = true,
    val textColor: String = "black",
    /** Varias páginas, cada una con su posición (como el editor de escritorio). */
    val perPage: Boolean = false,
    /** Depende del documento abierto: no se guarda en las preferencias. */
    val placements: Map<Int, SealPlacement> = emptyMap(),
    val csvEnabled: Boolean = false,
    /** El CSV lo emite la Administración para cada documento: no se guarda. */
    val csvCode: String = "",
    val csvUrl: String = "",
    val csvText: String = "",
    val csvQr: Boolean = true,
) {
    /** Guarda la posición actual como la de [number] y la incluye en la lista. */
    fun withPagePlacement(number: Int): SealSettings =
        copy(placements = placements + (number to SealPlacement(rect, rotation)))

    fun withoutPage(number: Int): SealSettings = copy(placements = placements - number)

    /** Al cambiar de página se edita la posición propia de esa página, si la tiene. */
    fun loadPage(number: Int): SealSettings {
        val stored = placements[number] ?: return copy(page = number)
        return copy(page = number, rect = stored.rect, rotation = stored.rotation)
    }

    /** Lista de páginas y posiciones que recibirá el motor. */
    fun placementList(pageCount: Int): List<Pair<Int, SealPlacement>> = when {
        perPage -> {
            require(placements.isNotEmpty() && placements.size <= MAX_PLACEMENTS)
            require(placements.all { (number, placement) -> number in 1..pageCount && placement.valid() })
            placements.toSortedMap().toList()
        }
        allPages -> {
            require(pageCount <= MAX_PLACEMENTS)
            (1..pageCount).map { it to SealPlacement(rect, rotation) }
        }
        else -> listOf(page to SealPlacement(rect, rotation))
    }

    fun options(pageCount: Int, pageWidth: Int, pageHeight: Int, imageBase64: String? = null): Map<String, String> {
        require(enabled && rect.valid() && rotation in 0..359 && opacity in 0..100)
        require(pageCount >= 1 && page in 1..pageCount && (!allPages || pageCount <= MAX_PLACEMENTS))
        require(pageWidth > 0 && pageHeight > 0)
        require(logo in setOf("none", "institutional", "custom"))
        require(textColor in setOf("black", "blue", "darkgray"))
        require(keepText || logo != "none" || qrEnabled)
        val placements = JSONArray()
        for ((number, placement) in placementList(pageCount)) {
            placements.put(JSONObject()
                .put("page", number)
                .put("rect", JSONObject()
                    .put("x", placement.rect.x.toDouble())
                    .put("y", placement.rect.y.toDouble())
                    .put("w", placement.rect.w.toDouble())
                    .put("h", placement.rect.h.toDouble()))
                .put("rotation", placement.rotation))
        }
        return buildMap {
            put("visibleSeal", "true")
            put("visibleSealPlacements", placements.toString())
            put("visibleSealRectW", (rect.w * pageWidth).toString())
            put("visibleSealRectH", (rect.h * pageHeight).toString())
            put("visibleSealLogoOpacityPercent", opacity.toString())
            put("visibleSealKeepText", keepText.toString())
            put("layer2FontColor", textColor)
            if (logo == "institutional") put("visibleSealLogo", "institucional")
            if (logo == "custom") {
                require(!imageBase64.isNullOrBlank())
                put("visibleSealImageBase64", imageBase64)
            }
            if (qrEnabled) put("qrContent", normalizedHttps(qrAddress))
            putAll(csvOptions())
        }
    }

    /** Opciones de la leyenda CSV del motor; la URL la normaliza el motor. */
    fun csvOptions(): Map<String, String> {
        if (!csvEnabled) return emptyMap()
        val code = csvCode.trim()
        val url = csvUrl.trim()
        val text = csvText.trim()
        require(code.isNotEmpty() && code.length <= MAX_CSV_CODE && code.none(Char::isISOControl))
        require(url.isNotEmpty() && url.length <= MAX_CSV_URL)
        require(text.length <= MAX_CSV_TEXT && text.none(Char::isISOControl))
        return buildMap {
            put("csv", code)
            put("csvUrl", url)
            if (text.isNotEmpty()) put("csvText", text)
            put("csvQR", csvQr.toString())
        }
    }

    companion object {
        const val MAX_PLACEMENTS = 128
        const val MAX_CSV_CODE = 128
        const val MAX_CSV_URL = 2048
        const val MAX_CSV_TEXT = 512
    }
}

fun normalizedHttps(raw: String): String {
    val value = raw.trim()
    require(value.isNotBlank() && value.length <= 2048 && value.none(Char::isWhitespace))
    val complete = if (value.contains("://")) value else "https://$value"
    val uri = URI(complete)
    require(uri.scheme.equals("https", ignoreCase = true) && !uri.host.isNullOrBlank() && uri.userInfo == null)
    return complete
}

/** X/Y del motor se miden desde abajo; el lienzo Android se mide desde arriba. */
object SealGeometry {
    fun fromScreen(left: Float, top: Float, width: Float, height: Float, canvasWidth: Float, canvasHeight: Float): SealRect {
        require(canvasWidth > 0f && canvasHeight > 0f)
        return SealRect(left / canvasWidth, 1f - (top + height) / canvasHeight, width / canvasWidth, height / canvasHeight)
    }

    fun fromScreenWithRotation(
        left: Float, top: Float, width: Float, height: Float,
        canvasWidth: Float, canvasHeight: Float, angle: Int,
    ): SealRect = fit(fromScreen(left, top, width, height, canvasWidth, canvasHeight), angle, canvasWidth / canvasHeight)

    fun rotatedBounds(rect: SealRect, angle: Int, pageAspect: Float = 1f): Pair<Float, Float> {
        require(pageAspect > 0f && pageAspect.isFinite())
        val radians = Math.toRadians(angle.toDouble())
        return Pair(
            (abs(rect.w * cos(radians)) + abs(rect.h / pageAspect * sin(radians))).toFloat(),
            (abs(rect.w * pageAspect * sin(radians)) + abs(rect.h * cos(radians))).toFloat(),
        )
    }

    fun fit(rect: SealRect, angle: Int, pageAspect: Float = 1f): SealRect {
        val (boundW, boundH) = rotatedBounds(rect, angle, pageAspect)
        require(boundW <= 1f && boundH <= 1f)
        val halfW = maxOf(boundW, rect.w) / 2f
        val halfH = maxOf(boundH, rect.h) / 2f
        val cx = (rect.x + rect.w / 2f).coerceIn(halfW, 1f - halfW)
        val cy = (rect.y + rect.h / 2f).coerceIn(halfH, 1f - halfH)
        return rect.copy(x = cx - rect.w / 2f, y = cy - rect.h / 2f)
    }

    fun snap(angle: Int): Int {
        val normalized = ((angle % 360) + 360) % 360
        val nearest = ((normalized + 45) / 90 * 90) % 360
        val distance = minOf(abs(normalized - nearest), 360 - abs(normalized - nearest))
        return if (distance <= 7) nearest else normalized
    }
}

class SealPreferences(context: Context) {
    private val preferences = context.getSharedPreferences("visible_seal", Context.MODE_PRIVATE)

    fun load(): SealSettings = SealSettings(
        enabled = preferences.getBoolean("enabled", false),
        rect = SealRect(
            preferences.getFloat("x", 0.60f), preferences.getFloat("y", 0.06f),
            preferences.getFloat("w", 0.32f), preferences.getFloat("h", 0.11f),
        ).let { if (it.valid()) it else SealRect() },
        rotation = preferences.getInt("rotation", 0).takeIf { it in 0..359 } ?: 0,
        opacity = preferences.getInt("opacity", 100).coerceIn(0, 100),
        page = preferences.getInt("page", 1).coerceAtLeast(1),
        allPages = preferences.getBoolean("all_pages", false),
        perPage = preferences.getBoolean("per_page", false),
        qrEnabled = preferences.getBoolean("qr_enabled", false),
        qrAddress = preferences.getString("qr_address", "").orEmpty(),
        logo = preferences.getString("logo", "none").orEmpty().takeIf { it in setOf("none", "institutional", "custom") } ?: "none",
        keepText = preferences.getBoolean("keep_text", true),
        textColor = preferences.getString("text_color", "black").orEmpty().takeIf {
            it in setOf("black", "blue", "darkgray")
        } ?: "black",
        csvEnabled = preferences.getBoolean("csv_enabled", false),
        csvUrl = preferences.getString("csv_url", "").orEmpty().take(SealSettings.MAX_CSV_URL),
        csvText = preferences.getString("csv_text", "").orEmpty().take(SealSettings.MAX_CSV_TEXT),
        csvQr = preferences.getBoolean("csv_qr", true),
    )

    /** Vuelve a los valores predeterminados del sello. */
    fun clear() {
        preferences.edit { clear() }
    }

    fun save(settings: SealSettings) {
        preferences.edit {
            putBoolean("enabled", settings.enabled)
            putFloat("x", settings.rect.x).putFloat("y", settings.rect.y)
            putFloat("w", settings.rect.w).putFloat("h", settings.rect.h)
            putInt("rotation", settings.rotation).putInt("opacity", settings.opacity)
            putInt("page", settings.page).putBoolean("all_pages", settings.allPages)
            putBoolean("qr_enabled", settings.qrEnabled).putString("qr_address", settings.qrAddress)
            putString("logo", settings.logo)
            putBoolean("keep_text", settings.keepText).putString("text_color", settings.textColor)
            putBoolean("per_page", settings.perPage)
            putBoolean("csv_enabled", settings.csvEnabled).putString("csv_url", settings.csvUrl)
            putString("csv_text", settings.csvText).putBoolean("csv_qr", settings.csvQr)
        }
    }
}
