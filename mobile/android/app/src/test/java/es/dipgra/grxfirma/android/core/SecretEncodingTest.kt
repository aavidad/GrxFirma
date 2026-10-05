// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.core

import org.junit.Assert.*
import org.junit.Test

class SecretEncodingTest {
    @Test fun `password conversion uses mutable UTF8 bytes and preserves Unicode`() {
        val secret = charArrayOf('ñ', 'a', '密')
        val bytes = SecretEncoding.utf8(secret)
        assertArrayEquals("ña密".toByteArray(Charsets.UTF_8), bytes)
        bytes.fill(0)
        secret.fill('\u0000')
        assertArrayEquals(ByteArray(6), bytes)
        assertArrayEquals(CharArray(3), secret)
    }

    @Test fun `oversized and malformed passwords are rejected`() {
        assertThrows(IllegalArgumentException::class.java) { SecretEncoding.utf8(CharArray(1025) { 'a' }) }
        assertThrows(java.nio.charset.CharacterCodingException::class.java) { SecretEncoding.utf8(charArrayOf('\uD800')) }
    }
}
