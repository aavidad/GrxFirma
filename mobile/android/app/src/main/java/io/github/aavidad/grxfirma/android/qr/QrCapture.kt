// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.qr

import android.content.Context
import android.net.Uri
import androidx.core.content.FileProvider
import java.io.File
import java.io.RandomAccessFile

/**
 * Fichero temporal privado donde la app de cámara deja la foto del QR. Se
 * comparte solo con esa app, con permiso de escritura puntual
 * (ACTION_IMAGE_CAPTURE), así que GrxFirma no necesita el permiso CAMERA.
 * La foto se borra en cuanto se ha leído, si se cancela y al abrir la app.
 */
object QrCapture {
    const val DIRECTORY = "qr-capture"
    private const val FILE_NAME = "qr.jpg"

    fun authority(context: Context): String = "${context.packageName}.qrcapture"

    fun file(context: Context): File = File(File(context.cacheDir, DIRECTORY), FILE_NAME)

    /** Prepara un fichero vacío y devuelve su URI content:// para la cámara. */
    fun prepare(context: Context): Uri {
        clear(context)
        val target = file(context)
        val directory = target.parentFile ?: throw IllegalStateException("QR_CAPTURE_DIRECTORY")
        if (!directory.isDirectory && !directory.mkdirs()) throw IllegalStateException("QR_CAPTURE_DIRECTORY")
        if (!target.createNewFile()) throw IllegalStateException("QR_CAPTURE_FILE")
        return FileProvider.getUriForFile(context, authority(context), target)
    }

    /** Sobrescribe y borra todo lo que haya en el directorio de capturas. */
    fun clear(context: Context) {
        File(context.cacheDir, DIRECTORY).listFiles()?.forEach(::wipe)
    }

    fun wipe(file: File) {
        try {
            if (file.isFile) {
                RandomAccessFile(file, "rw").use { raf ->
                    val zeros = ByteArray(64 * 1024)
                    var left = raf.length()
                    raf.seek(0)
                    while (left > 0) {
                        val chunk = minOf(left, zeros.size.toLong()).toInt()
                        raf.write(zeros, 0, chunk)
                        left -= chunk
                    }
                }
            }
        } catch (_: Exception) {
            // El borrado sigue aunque no se pueda sobrescribir.
        }
        file.delete()
    }
}
