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
            throw InvalidDocumentException(DocumentProblem.TOO_LARGE)
        }
    }

    /** Crea un documento nuevo dentro de una carpeta elegida con SAF. */
    fun writeToTree(folder: Uri, displayName: String, mimeType: String, bytes: ByteArray) {
        throw InvalidDocumentException(DocumentProblem.TREE_UNSUPPORTED)
    }

    /**
     * Ficheros (no subcarpetas) de una carpeta elegida con SAF, hasta
     * [maximumEntries]. El permiso de la carpeta no se persiste.
     */
    fun listTree(folder: Uri, maximumEntries: Int): List<SelectedFile> {
        throw InvalidDocumentException(DocumentProblem.TREE_UNSUPPORTED)
    }
}

open class ContentRepository(private val resolver: ContentResolver) : DocumentRepository {
    override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile {
        require(uri.scheme == ContentResolver.SCHEME_CONTENT)
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
    )

    override fun loadBounded(file: SelectedFile, maximumBytes: Int): LoadedFile =
        load(file = file, maximumBytes = maximumBytes)

    override fun loadCertificate(file: SelectedFile): LoadedFile = load(
        file = file,
        maximumBytes = DocumentPolicy.MAX_CERTIFICATE_BYTES,
    )

    override fun write(uri: Uri, bytes: ByteArray) {
        require(uri.scheme == ContentResolver.SCHEME_CONTENT)
        resolver.openOutputStream(uri, "w")?.use { output ->
            output.write(bytes)
            output.flush()
        } ?: throw InvalidDocumentException(DocumentProblem.DESTINATION_UNAVAILABLE)
    }

    override fun writeToTree(folder: Uri, displayName: String, mimeType: String, bytes: ByteArray) {
        require(folder.scheme == ContentResolver.SCHEME_CONTENT)
        val parent = DocumentsContract.buildDocumentUriUsingTree(folder, DocumentsContract.getTreeDocumentId(folder))
        val created = DocumentsContract.createDocument(
            resolver,
            parent,
            sanitizeMimeType(mimeType, "application/octet-stream"),
            DocumentPolicy.sanitizeDisplayName(displayName, "documento"),
        ) ?: throw InvalidDocumentException(DocumentProblem.CREATE_FAILED)
        write(created, bytes)
    }

    override fun listTree(folder: Uri, maximumEntries: Int): List<SelectedFile> {
        require(folder.scheme == ContentResolver.SCHEME_CONTENT)
        val children = DocumentsContract.buildChildDocumentsUriUsingTree(folder, DocumentsContract.getTreeDocumentId(folder))
        val columns = arrayOf(
            DocumentsContract.Document.COLUMN_DOCUMENT_ID,
            DocumentsContract.Document.COLUMN_DISPLAY_NAME,
            DocumentsContract.Document.COLUMN_MIME_TYPE,
            DocumentsContract.Document.COLUMN_SIZE,
        )
        val files = ArrayList<SelectedFile>()
        resolver.query(children, columns, null, null, null)?.use { cursor ->
            while (cursor.moveToNext()) {
                if (files.size >= maximumEntries) {
                    throw InvalidDocumentException(DocumentProblem.TOO_MANY_ENTRIES)
                }
                val id = cursor.optionalString(DocumentsContract.Document.COLUMN_DOCUMENT_ID) ?: continue
                val mime = cursor.optionalString(DocumentsContract.Document.COLUMN_MIME_TYPE)
                if (mime == DocumentsContract.Document.MIME_TYPE_DIR) continue
                files += SelectedFile(
                    uri = DocumentsContract.buildDocumentUriUsingTree(folder, id),
                    displayName = DocumentPolicy.sanitizeDisplayName(
                        cursor.optionalString(DocumentsContract.Document.COLUMN_DISPLAY_NAME), "documento.xml"),
                    mimeType = sanitizeMimeType(mime, "application/octet-stream"),
                    sizeBytes = cursor.optionalLong(DocumentsContract.Document.COLUMN_SIZE),
                )
            }
        } ?: throw InvalidDocumentException(DocumentProblem.FOLDER_UNREADABLE)
        return files
    }

    private fun load(file: SelectedFile, maximumBytes: Int): LoadedFile {
        DocumentPolicy.requireAllowedSize(file.sizeBytes, maximumBytes)
        val bytes = resolver.openInputStream(file.uri)?.use { input ->
            DocumentPolicy.readBounded(input, maximumBytes)
        } ?: throw InvalidDocumentException(DocumentProblem.SOURCE_UNREADABLE)
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
