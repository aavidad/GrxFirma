// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import androidx.annotation.StringRes
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.DocumentServices
import io.github.aavidad.grxfirma.android.core.Wave4Capabilities
import io.github.aavidad.grxfirma.android.files.DocumentPolicy
import io.github.aavidad.grxfirma.android.model.SelectedFile
import java.util.Locale

/**
 * Estado de la cuarta oleada: expediente ENI y opciones del lote. Vive en su
 * propio tipo para no mezclarse con el resto de MainUiState.
 */
data class Wave4State(
    /** Documentos ENI XML de la carpeta elegida, en orden alfabético. */
    val eniFileDocuments: List<SelectedFile> = emptyList(),
    /** Ficheros de la carpeta que no son XML y no entran en el expediente. */
    val eniFileSkipped: Int = 0,
    /** Fecha de apertura elegida en el calendario (medianoche UTC) o null. */
    val eniFileOpeningDate: Long? = null,
    /** Operación del lote: firma o cofirma, como en escritorio. */
    val batchAction: String = "sign",
    /** Añadir a los PDF del lote el sello visible configurado. */
    val batchSeal: Boolean = false,
)

val MainUiState.eniFileAvailable: Boolean
    get() = backend.available && DocumentServices.ENI_FILE in documentServices

val MainUiState.canCreateEniFile: Boolean
    get() = eniFileAvailable && canReplaceSelection && certificate != null && wave4.eniFileDocuments.isNotEmpty()

val MainUiState.batchCosignAvailable: Boolean get() = Wave4Capabilities.BATCH_COSIGN in capabilities
val MainUiState.batchSealAvailable: Boolean get() = Wave4Capabilities.BATCH_VISIBLE_SEAL in capabilities
val MainUiState.externalBatchAvailable: Boolean get() = Wave4Capabilities.EXTERNAL_BATCH in capabilities
val MainUiState.externalProtectSignAvailable: Boolean get() = Wave4Capabilities.EXTERNAL_PROTECT_SIGN in capabilities

/** Reglas del expediente ENI que no dependen de la pantalla. */
object ExpedientePolicy {
    /** Igual que el núcleo móvil (el motor de escritorio admite 128). */
    const val MAX_DOCUMENTS = 64
    const val MAX_TOTAL_BYTES: Long = DocumentPolicy.MAX_DOCUMENT_BYTES.toLong()
    /** Entradas de la carpeta que se examinan como máximo. */
    const val MAX_FOLDER_ENTRIES = 512
    const val MAX_INTERESTED = 16
    const val MAX_INTERESTED_CHARS = 128
    val STATES = listOf("E01", "E02", "E03")
    private val CLASSIFICATION = Regex("[0-9]{1,30}|[A-Z][0-9]{8}_PRO_[A-Za-z0-9_]{1,30}")
    private val IDENTIFIER = Regex("ES_[A-Z][0-9]{8}_[0-9]{4}_[A-Za-z0-9_]{1,30}")

    fun isXml(name: String, mimeType: String): Boolean =
        name.lowercase(Locale.ROOT).endsWith(".xml") || mimeType == "application/xml" || mimeType == "text/xml"

    /** Separa los XML (ordenados por nombre, como escritorio) del resto. */
    fun partition(entries: List<SelectedFile>): Pair<List<SelectedFile>, Int> {
        val order = xmlOrder(entries.map { it.displayName to it.mimeType })
        return order.map { entries[it] } to (entries.size - order.size)
    }

    /** Posiciones de los XML ordenadas por nombre. */
    fun xmlOrder(entries: List<Pair<String, String>>): List<Int> =
        entries.indices.filter { isXml(entries[it].first, entries[it].second) }.sortedBy { entries[it].first }

    @StringRes
    fun selectionProblem(sizes: List<Long?>): Int? = when {
        sizes.isEmpty() -> R.string.expediente_error_no_xml
        sizes.size > MAX_DOCUMENTS -> R.string.expediente_error_too_many
        sizes.any { it != null && it > DocumentPolicy.MAX_DOCUMENT_BYTES } -> R.string.expediente_error_too_large
        sizes.sumOf { it ?: 0L } > MAX_TOTAL_BYTES -> R.string.expediente_error_too_large
        else -> null
    }

    fun classificationValid(value: String): Boolean = CLASSIFICATION.matches(value.trim())

    fun identifierValid(value: String): Boolean = value.isBlank() || IDENTIFIER.matches(value.trim())

    /** Interesados separados por punto y coma o saltos de línea (un NIF puede llevar comas en el nombre). */
    fun interested(raw: String): List<String> = raw.split(';', '\n')
        .map { it.trim() }
        .filter { it.isNotEmpty() }
        .distinct()

    fun interestedValid(values: List<String>): Boolean =
        values.size <= MAX_INTERESTED && values.all { it.length <= MAX_INTERESTED_CHARS && it.none(Char::isISOControl) }

    fun outputName(identifier: String): String {
        val base = identifier.trim().ifBlank { "expediente" }.take(120)
        return DocumentPolicy.sanitizeDisplayName("$base.eni.xml", "expediente.eni.xml")
    }
}

/**
 * Opciones del sello visible para un PDF del lote, calculadas con su número
 * de páginas y tamaño. Devuelve null si el PDF no se puede preparar.
 */
fun interface BatchSealPlanner {
    fun options(file: io.github.aavidad.grxfirma.android.model.LoadedFile): Map<String, String>?
}
