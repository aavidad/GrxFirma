// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.qr

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import java.io.ByteArrayOutputStream

/**
 * Prepara la imagen del QR tributario para el núcleo, que lee PNG o JPEG de
 * hasta 20 MiB y 36 megapíxeles. Las fotos de muchos móviles superan esos
 * límites y la galería puede dar HEIC o WebP: en esos casos se reduce con
 * BitmapFactory (sin decodificar a tamaño completo) y se vuelve a codificar
 * en JPEG. Con 6000 píxeles de lado como mucho el QR sigue siendo legible
 * y la imagen cabe en los límites del núcleo.
 */
object QrImagePreparer {
    /** Lo más que se lee del fichero elegido o de la foto. */
    const val MAX_SOURCE_BYTES = 32 * 1024 * 1024
    const val MAX_CORE_BYTES = 20 * 1024 * 1024
    const val MAX_CORE_PIXELS = 36_000_000L
    const val MAX_CORE_SIDE = 12_000
    const val TARGET_SIDE = 6_000
    private const val JPEG_QUALITY = 92

    fun isPngOrJpeg(bytes: ByteArray): Boolean =
        (bytes.size >= 8 && bytes[0] == 0x89.toByte() && bytes[1] == 'P'.code.toByte() &&
            bytes[2] == 'N'.code.toByte() && bytes[3] == 'G'.code.toByte()) ||
            (bytes.size >= 3 && bytes[0] == 0xFF.toByte() && bytes[1] == 0xD8.toByte() && bytes[2] == 0xFF.toByte())

    /** ¿Hay que reducir o convertir la imagen antes de dársela al núcleo? */
    fun needsConversion(width: Int, height: Int, sizeBytes: Int, pngOrJpeg: Boolean): Boolean =
        !pngOrJpeg || sizeBytes > MAX_CORE_BYTES || width > MAX_CORE_SIDE || height > MAX_CORE_SIDE ||
            width.toLong() * height.toLong() > MAX_CORE_PIXELS

    /** Potencia de dos que deja el lado mayor en [TARGET_SIDE] píxeles o menos. */
    fun sampleSize(width: Int, height: Int): Int {
        var sample = 1
        val side = maxOf(width, height)
        while (side / sample > TARGET_SIDE && sample < 256) sample *= 2
        return sample
    }

    /**
     * Devuelve los bytes que recibe el núcleo. Si convierte, borra [source].
     * Si Android no reconoce la imagen la deja tal cual: el núcleo la rechaza
     * con su mensaje.
     */
    fun prepare(source: ByteArray): ByteArray {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(source, 0, source.size, bounds)
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return source
        if (!needsConversion(bounds.outWidth, bounds.outHeight, source.size, isPngOrJpeg(source))) return source
        // RGB_565 basta para un QR (blanco y negro) y usa la mitad de memoria.
        val options = BitmapFactory.Options().apply {
            inSampleSize = sampleSize(bounds.outWidth, bounds.outHeight)
            inPreferredConfig = Bitmap.Config.RGB_565
        }
        val bitmap = try {
            BitmapFactory.decodeByteArray(source, 0, source.size, options)
        } catch (_: OutOfMemoryError) {
            null
        } ?: return source
        return try {
            ByteArrayOutputStream().use { output ->
                bitmap.compress(Bitmap.CompressFormat.JPEG, JPEG_QUALITY, output)
                output.toByteArray()
            }.also { source.fill(0) }
        } finally {
            bitmap.recycle()
        }
    }
}
