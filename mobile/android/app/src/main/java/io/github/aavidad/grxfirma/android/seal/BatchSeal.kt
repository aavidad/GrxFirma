// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.seal

import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.ui.BatchSealPlanner
import java.io.File
import java.nio.ByteBuffer
import java.nio.channels.FileChannel
import java.nio.file.Files
import java.nio.file.LinkOption
import java.nio.file.Path
import java.nio.file.StandardOpenOption

/**
 * Sello visible en el lote. Se usa el sello guardado en «Firma visible»
 * (posición, estilo y páginas) adaptado a cada PDF. La leyenda CSV no se
 * aplica: su código es propio de cada documento.
 */
object BatchSeal {
    /** Subdirectorio de noBackupFilesDir con las copias temporales de cada PDF. */
    const val WORK_DIRECTORY = ".grxfirma-lote"

    /** Bloque de ceros con que se sobrescriben las copias antes de borrarlas. */
    private const val WIPE_BLOCK = 64 * 1024

    /**
     * Vacía las copias que pudieran quedar de un cierre inesperado. Se llama al
     * abrir la app y al empezar cada lote. Solo toca ficheros regulares del
     * primer nivel, nunca sigue enlaces ni entra en subdirectorios, y un fallo
     * no se propaga: no debe impedir que la app arranque.
     */
    fun clearWorkDirectory(directory: File) {
        try {
            val root = directory.toPath()
            if (!Files.isDirectory(root, LinkOption.NOFOLLOW_LINKS)) return
            Files.newDirectoryStream(root).use { entries ->
                for (entry in entries) {
                    try {
                        if (!Files.isRegularFile(entry, LinkOption.NOFOLLOW_LINKS)) continue
                        wipe(entry)
                        Files.deleteIfExists(entry)
                    } catch (_: Exception) {
                        // Se sigue con el resto; la copia se intentará borrar en el próximo arranque.
                    }
                }
            }
        } catch (_: Exception) {
        }
    }

    /** Sobrescribe con ceros por bloques, sin cargar el fichero en memoria. */
    private fun wipe(path: Path) {
        FileChannel.open(path, StandardOpenOption.WRITE, LinkOption.NOFOLLOW_LINKS).use { channel ->
            val zeros = ByteBuffer.allocate(WIPE_BLOCK)
            var remaining = channel.size()
            channel.position(0)
            while (remaining > 0) {
                zeros.clear()
                if (remaining < WIPE_BLOCK) zeros.limit(remaining.toInt())
                remaining -= channel.write(zeros)
            }
            channel.force(true)
        }
    }

    /** Ajusta las páginas del sello al número de páginas de un PDF concreto. */
    fun settingsFor(base: SealSettings, pageCount: Int): SealSettings {
        require(pageCount >= 1)
        val common = base.copy(enabled = true, csvEnabled = false, csvCode = "")
        return when {
            base.perPage -> {
                val kept = base.placements.filterKeys { it in 1..pageCount }
                if (kept.isNotEmpty()) common.copy(placements = kept)
                else common.copy(perPage = false, placements = emptyMap(), page = base.page.coerceIn(1, pageCount))
            }
            base.allPages && pageCount > SealSettings.MAX_PLACEMENTS ->
                throw IllegalArgumentException("TOO_MANY_PAGES")
            base.allPages -> common.copy(page = 1)
            // Si el PDF tiene menos páginas que la elegida, el sello va en la última.
            else -> common.copy(page = base.page.coerceIn(1, pageCount))
        }
    }

    /** Página cuyo tamaño se usa para medir el sello. */
    fun referencePage(settings: SealSettings): Int =
        if (settings.perPage) settings.placements.keys.minOrNull() ?: 1 else settings.page
}

/**
 * Lee con PdfRenderer el número de páginas y el tamaño de la página de
 * referencia. El PDF se copia un instante a un fichero privado sin copia de
 * seguridad, porque PdfRenderer necesita un descriptor con acceso aleatorio, y
 * se borra al terminar.
 */
class PdfBatchSealPlanner(
    private val workDirectory: File,
    private val settings: SealSettings,
    private val imageBase64: String?,
) : BatchSealPlanner {
    init {
        // Cada lote empieza sin copias de lotes anteriores.
        BatchSeal.clearWorkDirectory(workDirectory)
    }

    override fun options(file: LoadedFile): Map<String, String>? {
        if (!workDirectory.isDirectory && !workDirectory.mkdirs()) return null
        val temporary = try { File.createTempFile("lote", ".pdf", workDirectory) } catch (_: Exception) { return null }
        return try {
            temporary.writeBytes(file.bytes)
            ParcelFileDescriptor.open(temporary, ParcelFileDescriptor.MODE_READ_ONLY).use { descriptor ->
                PdfRenderer(descriptor).use { renderer ->
                    val count = renderer.pageCount
                    if (count !in 1..2048) return null
                    val adapted = BatchSeal.settingsFor(settings, count)
                    val pageIndex = (BatchSeal.referencePage(adapted) - 1).coerceIn(0, count - 1)
                    renderer.openPage(pageIndex).use { page ->
                        adapted.options(count, page.width, page.height, imageBase64)
                    }
                }
            }
        } catch (_: Exception) {
            null
        } finally {
            // El fichero contiene el documento: se sobrescribe antes de borrarlo.
            try { temporary.writeBytes(ByteArray(temporary.length().toInt().coerceAtMost(file.bytes.size))) } catch (_: Exception) { }
            temporary.delete()
        }
    }
}
