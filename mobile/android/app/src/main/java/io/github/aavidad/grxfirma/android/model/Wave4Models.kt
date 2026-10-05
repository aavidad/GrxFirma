// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.model

/** Metadatos del expediente ENI; la fecha de apertura va en RFC 3339. */
data class EniFileRequest(
    val organs: List<String>,
    val classification: String,
    val state: String,
    val identifier: String = "",
    val openingDate: String = "",
    val interested: List<String> = emptyList(),
)

/**
 * Resultado del expediente: el XML firmado o, si algún fichero no es un
 * documento ENI, las incidencias por fichero (sin firma).
 */
class EniFileResult(val bytes: ByteArray?, val documents: Int, val issues: List<EngineIssue>)

/** Documento del lote con su operación y opciones propias (sello visible). */
class BatchItemInput(
    val file: LoadedFile,
    val format: String,
    val action: String,
    val options: Map<String, String> = emptyMap(),
)
