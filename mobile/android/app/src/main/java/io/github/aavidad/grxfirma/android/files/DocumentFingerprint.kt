// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.files

import java.security.MessageDigest

/**
 * Huella SHA-256 del documento elegido (H-06). Se calcula al elegirlo y se
 * vuelve a comprobar al firmar, para no firmar otro contenido si el proveedor
 * que lo compartió lo cambia entre medias.
 */
object DocumentFingerprint {
    /** Caracteres hexadecimales que enseña el resumen del documento. */
    const val VISIBLE_PREFIX_LENGTH: Int = 16

    fun sha256Hex(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

    fun visiblePrefix(sha256: String): String = sha256.take(VISIBLE_PREFIX_LENGTH)

    /** Compara en tiempo constante la huella guardada con la de [bytes]. */
    fun matches(expectedHex: String, bytes: ByteArray): Boolean = MessageDigest.isEqual(
        expectedHex.lowercase().toByteArray(Charsets.US_ASCII),
        sha256Hex(bytes).toByteArray(Charsets.US_ASCII),
    )
}
