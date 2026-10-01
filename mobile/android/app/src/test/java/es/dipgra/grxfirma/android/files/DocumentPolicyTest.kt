// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.files

import java.io.ByteArrayInputStream
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class DocumentPolicyTest {
    @Test
    fun `readBounded reads content up to the configured limit`() {
        val content = "documento".encodeToByteArray()

        val actual = DocumentPolicy.readBounded(
            ByteArrayInputStream(content),
            content.size,
            "El documento",
        )

        assertArrayEquals(content, actual)
    }

    @Test
    fun `readBounded rejects content beyond the limit even without metadata`() {
        val error = assertThrows(InvalidDocumentException::class.java) {
            DocumentPolicy.readBounded(
                ByteArrayInputStream(ByteArray(33)),
                32,
                "El documento",
            )
        }

        assertEquals("El documento supera el límite de 0 MiB.", error.message)
    }

    @Test
    fun `readBounded rejects empty documents`() {
        assertThrows(InvalidDocumentException::class.java) {
            DocumentPolicy.readBounded(ByteArrayInputStream(byteArrayOf()), 32, "El documento")
        }
    }

    @Test
    fun `sanitizeDisplayName removes paths controls and excessive length`() {
        val actual = DocumentPolicy.sanitizeDisplayName(
            "../ruta/contrato\u0000" + "x".repeat(300) + ".pdf",
            "documento",
        )

        assertEquals(160, actual.length)
        assertEquals(false, actual.contains('/'))
        assertEquals(false, actual.any(Char::isISOControl))
    }
}
