// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import org.json.JSONArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class SealSettingsPagesTest {
    @Test fun `each page keeps its own position and rotation`() {
        var settings = SealSettings(enabled = true, perPage = true, rect = SealRect(0.1f, 0.1f, 0.3f, 0.1f))
        settings = settings.withPagePlacement(1)
        settings = settings.copy(rect = SealRect(0.5f, 0.7f, 0.3f, 0.1f), rotation = 90).withPagePlacement(3)
        val placements = JSONArray(settings.options(4, 595, 842).getValue("visibleSealPlacements"))
        assertEquals(2, placements.length())
        assertEquals(1, placements.getJSONObject(0).getInt("page"))
        assertEquals(0.1, placements.getJSONObject(0).getJSONObject("rect").getDouble("x"), 0.0001)
        assertEquals(3, placements.getJSONObject(1).getInt("page"))
        assertEquals(90, placements.getJSONObject(1).getInt("rotation"))
        val back = settings.loadPage(1)
        assertEquals(SealRect(0.1f, 0.1f, 0.3f, 0.1f), back.rect)
        assertEquals(0, back.rotation)
        assertThrows(IllegalArgumentException::class.java) { settings.withoutPage(1).withoutPage(3).options(4, 595, 842) }
        assertThrows(IllegalArgumentException::class.java) { settings.options(2, 595, 842) } // página 3 fuera del PDF
    }

    @Test fun `all pages and single page keep their previous behaviour`() {
        val all = SealSettings(enabled = true, allPages = true)
        assertEquals(3, JSONArray(all.options(3, 595, 842).getValue("visibleSealPlacements")).length())
        val one = SealSettings(enabled = true, page = 2)
        assertEquals(2, JSONArray(one.options(3, 595, 842).getValue("visibleSealPlacements")).getJSONObject(0).getInt("page"))
    }

    @Test fun `CSV legend adds the engine options only when complete`() {
        val base = SealSettings(enabled = true, csvEnabled = true, csvCode = " ABC-123 ", csvUrl = "sede.example/c?csv={csv}")
        val options = base.options(1, 595, 842)
        assertEquals("ABC-123", options["csv"])
        assertEquals("sede.example/c?csv={csv}", options["csvUrl"])
        assertEquals("true", options["csvQR"])
        assertFalse(options.containsKey("csvText"))
        assertThrows(IllegalArgumentException::class.java) { base.copy(csvCode = "").options(1, 595, 842) }
        assertThrows(IllegalArgumentException::class.java) { base.copy(csvText = "a\nb").options(1, 595, 842) }
        assertTrue(base.copy(csvEnabled = false).options(1, 595, 842).keys.none { it.startsWith("csv") })
    }
}
