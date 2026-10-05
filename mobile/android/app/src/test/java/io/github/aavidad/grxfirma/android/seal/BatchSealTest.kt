// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

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

    @Test fun `work directory cleanup removes only top level regular files without following links`() {
        val base = java.nio.file.Files.createTempDirectory("grxfirma-lote-test").toFile()
        try {
            val work = java.io.File(base, BatchSeal.WORK_DIRECTORY).apply { assertTrue(mkdirs()) }
            val outside = java.io.File(base, "fuera.pdf").apply { writeBytes(ByteArray(200_000) { 7 }) }
            val copy = java.io.File(work, "copia.pdf").apply { writeBytes(ByteArray(150_000) { 9 }) }
            val nested = java.io.File(work, "sub").apply { assertTrue(mkdirs()) }
            val nestedFile = java.io.File(nested, "dentro.pdf").apply { writeBytes(byteArrayOf(1)) }
            val link = java.io.File(work, "enlace.pdf").toPath()
            java.nio.file.Files.createSymbolicLink(link, outside.toPath())

            BatchSeal.clearWorkDirectory(work)

            assertFalse(copy.exists())
            assertTrue("el destino del enlace no se toca", outside.readBytes().all { it == 7.toByte() })
            assertTrue(java.nio.file.Files.isSymbolicLink(link))
            assertTrue("no entra en subdirectorios", nestedFile.exists())
            BatchSeal.clearWorkDirectory(java.io.File(base, "no-existe"))
            BatchSeal.clearWorkDirectory(outside)
            assertTrue(outside.exists())
        } finally {
            base.deleteRecursively()
        }
    }
}
