// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.files

import android.content.ContentResolver
import android.database.Cursor
import android.net.Uri
import android.provider.DocumentsContract
import android.provider.OpenableColumns
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.SelectedFile

interface DocumentRepository {
    fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile
    fun loadDocument(file: SelectedFile): LoadedFile
    fun loadCertificate(file: SelectedFile): LoadedFile
    fun write(uri: Uri, bytes: ByteArray)

    /** Lee un fichero con un límite propio (registros Veri*Factu, firmas para ENI). */
    fun loadBounded(file: SelectedFile, maximumBytes: Int): LoadedFile = loadDocument(file).also {
        if (it.bytes.size > maximumBytes) {
            it.bytes.fill(0)
            throw InvalidDocumentException("El documento supera el tamaño permitido.")
        }
    }

    /** Crea un documento nuevo dentro de una carpeta elegida con SAF. */
    fun writeToTree(folder: Uri, displayName: String, mimeType: String, bytes: ByteArray) {
        throw InvalidDocumentException("Este repositorio no admite carpetas.")
    }
}

open class ContentRepository(private val resolver: ContentResolver) : DocumentRepository {
    override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile {
        require(uri.scheme == ContentResolver.SCHEME_CONTENT) {
            "Solo se admiten URI content:// proporcionadas por Android."
        }
        var displayName: String? = null
        var size: Long? = null
        resolver.query(
            uri,
            arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE),
            null,
            null,
            null,
        )?.use { cursor ->
            if (cursor.moveToFirst()) {
                displayName = cursor.optionalString(OpenableColumns.DISPLAY_NAME)
                size = cursor.optionalLong(OpenableColumns.SIZE)
            }
        }
        val mime = sanitizeMimeType(resolver.getType(uri), fallbackMime)
        return SelectedFile(
            uri = uri,
            displayName = DocumentPolicy.sanitizeDisplayName(displayName, fallbackName),
            mimeType = mime,
            sizeBytes = size,
        )
    }

    override fun loadDocument(file: SelectedFile): LoadedFile = load(
        file = file,
        maximumBytes = DocumentPolicy.MAX_DOCUMENT_BYTES,
        label = "El documento",
    )

    override fun loadBounded(file: SelectedFile, maximumBytes: Int): LoadedFile =
        load(file = file, maximumBytes = maximumBytes, label = "El documento")

    override fun loadCertificate(file: SelectedFile): LoadedFile = load(
        file = file,
        maximumBytes = DocumentPolicy.MAX_CERTIFICATE_BYTES,
        label = "El certificado",
    )

    override fun write(uri: Uri, bytes: ByteArray) {
        require(uri.scheme == ContentResolver.SCHEME_CONTENT) {
            "El destino debe ser una URI content:// de Android."
        }
        resolver.openOutputStream(uri, "w")?.use { output ->
            output.write(bytes)
            output.flush()
        } ?: throw InvalidDocumentException("Android no ha permitido abrir el destino seleccionado.")
    }

    override fun writeToTree(folder: Uri, displayName: String, mimeType: String, bytes: ByteArray) {
        require(folder.scheme == ContentResolver.SCHEME_CONTENT) {
            "La carpeta debe ser una URI content:// de Android."
        }
        val parent = DocumentsContract.buildDocumentUriUsingTree(folder, DocumentsContract.getTreeDocumentId(folder))
        val created = DocumentsContract.createDocument(
            resolver,
            parent,
            sanitizeMimeType(mimeType, "application/octet-stream"),
            DocumentPolicy.sanitizeDisplayName(displayName, "documento"),
        ) ?: throw InvalidDocumentException("Android no ha permitido crear el fichero en la carpeta elegida.")
        write(created, bytes)
    }

    private fun load(file: SelectedFile, maximumBytes: Int, label: String): LoadedFile {
        DocumentPolicy.requireAllowedSize(file.sizeBytes, maximumBytes, label)
        val bytes = resolver.openInputStream(file.uri)?.use { input ->
            DocumentPolicy.readBounded(input, maximumBytes, label)
        } ?: throw InvalidDocumentException("Android no ha permitido abrir ${file.displayName}.")
        return LoadedFile(file.displayName, file.mimeType, bytes)
    }
}

private fun sanitizeMimeType(raw: String?, fallback: String): String {
    val candidate = raw.orEmpty().trim().lowercase().take(160)
    return if (candidate.matches(Regex("[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*"))) {
        candidate
    } else {
        fallback
    }
}

private fun Cursor.optionalString(column: String): String? {
    val index = getColumnIndex(column)
    return if (index >= 0 && !isNull(index)) getString(index) else null
}

private fun Cursor.optionalLong(column: String): Long? {
    val index = getColumnIndex(column)
    return if (index >= 0 && !isNull(index)) getLong(index) else null
}
