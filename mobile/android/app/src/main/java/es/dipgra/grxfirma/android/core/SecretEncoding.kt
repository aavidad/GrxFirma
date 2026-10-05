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

    /** Clave Base64 de EncryptedData: solo ASCII imprimible, hasta 64 caracteres. */
    fun ascii(secret: CharArray): ByteArray {
        require(secret.size in 1..64 && secret.all { it.code in 0x21..0x7e })
        return ByteArray(secret.size) { secret[it].code.toByte() }
    }
}
