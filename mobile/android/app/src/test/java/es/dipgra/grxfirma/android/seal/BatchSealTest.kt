// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.seal

import org.junit.Assert.*
import org.junit.Test

class BatchSealTest {
    @Test fun `chosen page falls back to the last page of shorter PDFs`() {
        val base = SealSettings(enabled = false, page = 5, csvEnabled = true, csvCode = "ABC")
        val short = BatchSeal.settingsFor(base, 2)
        assertTrue(short.enabled)
        assertEquals(2, short.page)
        assertFalse("el CSV es propio de cada documento", short.csvEnabled)
        assertEquals("", short.csvCode)
        assertEquals(5, BatchSeal.settingsFor(base, 9).page)
        assertTrue(short.options(2, 595, 842).getValue("visibleSealPlacements").contains("\"page\":2"))
    }

    @Test fun `all pages and per page placements adapt to each document`() {
        val all = SealSettings(allPages = true)
        assertEquals(3, BatchSeal.settingsFor(all, 3).placementList(3).size)
        assertThrows(IllegalArgumentException::class.java) { BatchSeal.settingsFor(all, SealSettings.MAX_PLACEMENTS + 1) }
        val placement = SealPlacement(SealRect(), 0)
        val perPage = SealSettings(perPage = true, placements = mapOf(1 to placement, 4 to placement))
        assertEquals(listOf(1), BatchSeal.settingsFor(perPage, 2).placementList(2).map { it.first })
        val none = BatchSeal.settingsFor(SealSettings(perPage = true, page = 3), 2)
        assertFalse(none.perPage)
        assertEquals(2, none.page)
        assertEquals(1, BatchSeal.referencePage(BatchSeal.settingsFor(perPage, 6)))
    }
}
