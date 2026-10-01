// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.files

import java.io.ByteArrayOutputStream
import java.io.InputStream

object DocumentPolicy {
    const val MAX_DOCUMENT_BYTES: Int = 32 * 1024 * 1024
    const val MAX_CERTIFICATE_BYTES: Int = 4 * 1024 * 1024
    const val MAX_SIGNED_OUTPUT_BYTES: Int = 48 * 1024 * 1024

    fun requireAllowedSize(sizeBytes: Long?, maximumBytes: Int, label: String) {
        if (sizeBytes != null && sizeBytes < 0) {
            throw InvalidDocumentException("El tamaño de $label no es válido.")
        }
        if (sizeBytes != null && sizeBytes > maximumBytes) {
            throw InvalidDocumentException(
                "$label supera el límite de ${maximumBytes / (1024 * 1024)} MiB.",
            )
        }
    }

    fun readBounded(input: InputStream, maximumBytes: Int, label: String): ByteArray {
        require(maximumBytes > 0) { "maximumBytes debe ser positivo" }
        val output = ByteArrayOutputStream(minOf(maximumBytes, DEFAULT_BUFFER_SIZE * 4))
        val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
        var total = 0
        while (true) {
            val read = input.read(buffer)
            if (read < 0) break
            if (read == 0) continue
            total += read
            if (total > maximumBytes) {
                buffer.fill(0)
                throw InvalidDocumentException(
                    "$label supera el límite de ${maximumBytes / (1024 * 1024)} MiB.",
                )
            }
            output.write(buffer, 0, read)
        }
        buffer.fill(0)
        return output.toByteArray().also {
            if (it.isEmpty()) throw InvalidDocumentException("$label está vacío.")
        }
    }

    fun sanitizeDisplayName(raw: String?, fallback: String): String {
        val normalized = raw
            .orEmpty()
            .substringAfterLast('/')
            .substringAfterLast('\\')
            .filterNot { it.isISOControl() }
            .trim()
            .take(160)
        return normalized.ifBlank { fallback }
    }
}

class InvalidDocumentException(message: String, cause: Throwable? = null) : Exception(message, cause)
