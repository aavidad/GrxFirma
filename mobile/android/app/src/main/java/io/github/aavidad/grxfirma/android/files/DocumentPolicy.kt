// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.files

import java.io.ByteArrayOutputStream
import java.io.InputStream

object DocumentPolicy {
    const val MAX_DOCUMENT_BYTES: Int = 32 * 1024 * 1024
    const val MAX_CERTIFICATE_BYTES: Int = 4 * 1024 * 1024
    const val MAX_SIGNED_OUTPUT_BYTES: Int = 48 * 1024 * 1024

    fun requireAllowedSize(sizeBytes: Long?, maximumBytes: Int) {
        if (sizeBytes != null && sizeBytes < 0) throw InvalidDocumentException(DocumentProblem.INVALID_SIZE)
        if (sizeBytes != null && sizeBytes > maximumBytes) throw InvalidDocumentException(DocumentProblem.TOO_LARGE)
    }

    fun readBounded(input: InputStream, maximumBytes: Int): ByteArray {
        require(maximumBytes > 0)
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
                throw InvalidDocumentException(DocumentProblem.TOO_LARGE)
            }
            output.write(buffer, 0, read)
        }
        buffer.fill(0)
        return output.toByteArray().also {
            if (it.isEmpty()) throw InvalidDocumentException(DocumentProblem.EMPTY)
        }
    }

    fun sanitizeDisplayName(raw: String?, fallback: String): String {
        val normalized = raw
            .orEmpty()
            .substringAfterLast('/')
            .substringAfterLast('\\')
            .filterNot(io.github.aavidad.grxfirma.android.core.DisplayText::hidden)
            .trim()
            .take(160)
        return normalized.ifBlank { fallback }
    }
}

/** Problemas de fichero con código cerrado; la interfaz los traduce con recursos. */
enum class DocumentProblem {
    INVALID_SIZE, TOO_LARGE, EMPTY, TREE_UNSUPPORTED, DESTINATION_UNAVAILABLE,
    CREATE_FAILED, TOO_MANY_ENTRIES, FOLDER_UNREADABLE, SOURCE_UNREADABLE,
}

class InvalidDocumentException(val problem: DocumentProblem, cause: Throwable? = null) : Exception(problem.name, cause)
