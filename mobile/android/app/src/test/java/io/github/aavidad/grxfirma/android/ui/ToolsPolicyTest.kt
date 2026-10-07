// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import java.security.SecureRandom
import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ToolsPolicyTest {
    @Test fun `batch limits match the mobile core`() {
        assertEquals(R.string.error_batch_required, ToolsPolicy.batchProblem(emptyList()))
        assertEquals(R.string.error_batch_too_many, ToolsPolicy.batchProblem(List(17) { 1L }))
        assertNull(ToolsPolicy.batchProblem(List(16) { 1024L }))
        val half = ToolsPolicy.MAX_BATCH_TOTAL_BYTES / 2
        assertNull(ToolsPolicy.batchProblem(listOf(half, half)))
        assertEquals(R.string.error_batch_too_large, ToolsPolicy.batchProblem(listOf(half, half + 1)))
        // Un tamaño desconocido se comprueba al leer, no al elegir.
        assertNull(ToolsPolicy.batchProblem(listOf(null, 1L)))
    }

    @Test fun `only canonical AES-256 Base64 keys are accepted`() {
        val canonical = Base64.getEncoder().encodeToString(ByteArray(32) { 7 }).toCharArray()
        assertTrue(ToolsPolicy.canonicalAesKey(canonical))
        assertFalse(ToolsPolicy.canonicalAesKey(canonical.copyOf(43)))
        val noncanonical = canonical.copyOf().also { it[42] = 'd' } // bits de relleno distintos de cero
        assertFalse(ToolsPolicy.canonicalAesKey(noncanonical))
        assertFalse(ToolsPolicy.canonicalAesKey(CharArray(44) { '*' }))
        assertFalse(ToolsPolicy.canonicalAesKey(Base64.getEncoder().encodeToString(ByteArray(33)).toCharArray()))
    }

    @Test fun `a transient key is rejected when invalid or when its copy differs`() {
        val key = Base64.getEncoder().encodeToString(ByteArray(32) { 3 }).toCharArray()
        assertFalse(ToolsPolicy.transientKeyRejected(key, key.copyOf()))
        assertTrue(ToolsPolicy.transientKeyRejected(key, key.copyOf().also { it[0] = 'B' }))
        assertTrue(ToolsPolicy.transientKeyRejected(CharArray(44) { '*' }, CharArray(44) { '*' }))
        // Sin documento el modelo avisa de eso primero: el campo de la clave no se marca.
        assertFalse(ToolsPolicy.keyFieldRejected(MainUiState(io.github.aavidad.grxfirma.android.core.CoreReadiness(true, "ready", ""),
            toolsAvailable = true, protectionContainer = "cms-encrypted"),
            CharArray(0), CharArray(0), sign = false))
    }

    @Test fun `generated keys are canonical and random`() {
        val random = SecureRandom()
        val first = ToolsPolicy.generateAesKey(random)
        val second = ToolsPolicy.generateAesKey(random)
        assertEquals(ToolsPolicy.AES_KEY_CHARS, first.size)
        assertTrue(ToolsPolicy.canonicalAesKey(first))
        assertFalse(first.contentEquals(second))
    }

    @Test fun `automatic format, names and MIME follow desktop`() {
        assertEquals("pades", ToolsPolicy.effectiveFormat("auto", "a.PDF", "application/octet-stream"))
        assertEquals("xades", ToolsPolicy.effectiveFormat("auto", "a.bin", "text/xml"))
        assertEquals("cades", ToolsPolicy.effectiveFormat("auto", "a.txt", "text/plain"))
        assertEquals("xades", ToolsPolicy.effectiveFormat("xades", "a.pdf", "application/pdf"))
        assertEquals("contrato.pdf.hexhash", ToolsPolicy.hashFileName("contrato.pdf", "hexhash"))
        // Con text/plain el selector añadiría «.txt» a «.hexhash».
        assertEquals("application/octet-stream", ToolsPolicy.HASH_MIME)
        assertEquals("ab12", ToolsPolicy.displayHash("hex", "ab12h"))
        assertEquals("q83vAA==", ToolsPolicy.displayHash("base64", "q83vAA=="))
        assertTrue(ToolsPolicy.looksUnprotectable("contrato.pdf.enveloped"))
        assertTrue(ToolsPolicy.looksUnprotectable("secreto.pdf.encrypted.p7m"))
        assertTrue(ToolsPolicy.looksUnprotectable("compartido"))
        assertFalse(ToolsPolicy.looksUnprotectable("contrato.pdf"))
        assertEquals(listOf("cms", "authenvelopeddata", "cms-encrypted"), ToolsPolicy.CONTAINERS)
    }

    @Test
    fun protectedFileNameIsShownAsProtectedFileNotBin() {
        assertTrue(ToolsPolicy.isProtectedFileName("documento.pdf.enveloped"))
        assertTrue(ToolsPolicy.isProtectedFileName("SOBRE.AFP"))
        assertFalse(ToolsPolicy.isProtectedFileName("firma.pdf.p7m"))
        assertFalse(ToolsPolicy.isProtectedFileName("contrato.pdf"))
    }
}
