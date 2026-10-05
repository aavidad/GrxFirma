// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import androidx.annotation.StringRes
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.files.DocumentPolicy
import java.security.SecureRandom
import java.util.Base64

/** Límites y reglas de las herramientas, iguales a los del núcleo móvil. */
object ToolsPolicy {
    const val MAX_BATCH_ITEMS = 16
    const val MAX_BATCH_TOTAL_BYTES: Long = DocumentPolicy.MAX_DOCUMENT_BYTES.toLong()
    const val MAX_RECIPIENTS = 16
    const val MAX_RECIPIENT_BYTES = 64 * 1024
    const val MAX_HASH_FILE_BYTES = 4 * 1024
    const val AES_KEY_CHARS = 44

    val HASH_ALGORITHMS = listOf("SHA-256", "SHA-1", "SHA-384", "SHA-512")
    val HASH_FORMATS = listOf("hex", "base64", "bin")
    val CONTAINERS = listOf("cms", "authenvelopeddata", "cms-encrypted")

    /** Devuelve el error del lote o null si cabe en los límites. */
    @StringRes
    fun batchProblem(sizes: List<Long?>): Int? = when {
        sizes.isEmpty() -> R.string.error_batch_required
        sizes.size > MAX_BATCH_ITEMS -> R.string.error_batch_too_many
        sizes.any { it == null || it < 0 } -> null
        sizes.sumOf { it ?: 0L } > MAX_BATCH_TOTAL_BYTES -> R.string.error_batch_too_large
        else -> null
    }

    /** Clave AES-256 en Base64 canónico: 44 caracteres que decodifican 32 bytes. */
    fun canonicalAesKey(secret: CharArray): Boolean {
        if (secret.size != AES_KEY_CHARS || secret.any { it.code > 0x7e }) return false
        val ascii = ByteArray(secret.size) { secret[it].code.toByte() }
        var decoded = ByteArray(0)
        var canonical = ByteArray(0)
        return try {
            decoded = Base64.getDecoder().decode(ascii)
            canonical = Base64.getEncoder().encode(decoded)
            decoded.size == 32 && canonical.contentEquals(ascii)
        } catch (_: IllegalArgumentException) {
            false
        } finally {
            ascii.fill(0)
            decoded.fill(0)
            canonical.fill(0)
        }
    }

    /** Genera una clave nueva; quien la recibe debe borrarla tras usarla. */
    fun generateAesKey(random: SecureRandom = SecureRandom()): CharArray {
        val key = ByteArray(32)
        val encoded: ByteArray
        try {
            random.nextBytes(key)
            encoded = Base64.getEncoder().encode(key)
        } finally {
            key.fill(0)
        }
        return try {
            CharArray(encoded.size) { encoded[it].toInt().toChar() }
        } finally {
            encoded.fill(0)
        }
    }

    /** Formato que aplicará el núcleo cuando se elige «automático». */
    fun effectiveFormat(format: String, name: String, mimeType: String, head: ByteArray? = null): String =
        if (format != "auto") format else FormatPolicy.detect(name, mimeType, head)

    const val MAX_VERIFACTU_FILES = 64
    const val MAX_VERIFACTU_FILE_BYTES = 10 * 1024 * 1024
    const val MAX_VERIFACTU_TOTAL_BYTES: Long = DocumentPolicy.MAX_DOCUMENT_BYTES.toLong()

    /** Devuelve el error de la selección de registros Veri*Factu o null. */
    @StringRes
    fun veriFactuProblem(sizes: List<Long?>): Int? = when {
        sizes.isEmpty() -> R.string.verifactu_none
        sizes.size > MAX_VERIFACTU_FILES -> R.string.error_verifactu_too_many
        sizes.any { it != null && it > MAX_VERIFACTU_FILE_BYTES } -> R.string.error_verifactu_too_many
        sizes.sumOf { it ?: 0L } > MAX_VERIFACTU_TOTAL_BYTES -> R.string.error_verifactu_too_many
        else -> null
    }

    fun hashFileName(documentName: String, extension: String): String =
        DocumentPolicy.sanitizeDisplayName("$documentName.$extension", "documento.$extension")

    /**
     * El selector de Android añade «.txt» a un fichero text/plain: la huella
     * («.hexhash», «.hashb64») se guarda siempre como binario para conservar su extensión.
     */
    const val HASH_MIME = "application/octet-stream"

    /** El fichero de huella de escritorio termina la huella hexadecimal en «h»; en pantalla no se muestra. */
    fun displayHash(format: String, hash: String): String =
        if (format == "hex" && hash.length > 1 && hash.endsWith("h") && hash.dropLast(1).all { it.isLetterOrDigit() }) hash.dropLast(1)
        else hash

    /** Extensiones de los ficheros protegidos que sabe abrir el núcleo (CMS y sobre .afp). */
    private val PROTECTED_EXTENSIONS = listOf(".enveloped", ".p7m", ".p7e", ".afp", ".cms")

    /**
     * Ficheros que solo produce «Proteger»; «.p7m» no entra porque también es
     * la extensión habitual de una firma CAdES.
     */
    fun isProtectedFileName(displayName: String): Boolean {
        val lower = displayName.lowercase(java.util.Locale.ROOT)
        return PROTECTED_EXTENSIONS.any { it != ".p7m" && lower.endsWith(it) }
    }

    /**
     * Si el nombre deja claro que el documento no está protegido, «Desproteger»
     * no puede funcionar. Sin extensión (compartido sin nombre) no se descarta.
     */
    fun looksUnprotectable(displayName: String): Boolean {
        val lower = displayName.lowercase(java.util.Locale.ROOT)
        val hasExtension = lower.substringAfterLast('/').contains('.')
        return !hasExtension || PROTECTED_EXTENSIONS.any { lower.endsWith(it) }
    }
}
