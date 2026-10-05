// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.core

import java.nio.CharBuffer
import java.nio.charset.CodingErrorAction

internal object SecretEncoding {
    fun utf8(secret: CharArray): ByteArray {
        val buffer = Charsets.UTF_8.newEncoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).encode(CharBuffer.wrap(secret))
        return try {
            require(buffer.remaining() in 1..1024)
            ByteArray(buffer.remaining()).also { buffer.get(it) }
        } finally { if (buffer.hasArray()) buffer.array().fill(0) }
    }
}
