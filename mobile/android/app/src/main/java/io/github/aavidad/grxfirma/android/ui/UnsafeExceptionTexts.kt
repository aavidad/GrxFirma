// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import android.content.Context

/** Solo claves propias de Android; los textos de cada caso pertenecen al núcleo. */
object UnsafeExceptionTexts {
    val keys = setOf("seccion", "introduccion", "activar", "desactivar", "activar_continuar", "cancelar",
        "mas_informacion", "cerrar", "actualizar", "cargando", "guardando", "no_disponible", "reenviar",
        "activada", "desactivada", "fijada_activa", "fijada_inactiva", "directiva_ilegible", "version",
        "aceptada_fecha", "desactivar_aqui", "ver_bloqueo", "version_no_disponible")
        .map { "android.inseguro.$it" }.toSet()
    fun text(context: Context, suffix: String) = EngineText.resolve(context, "android.inseguro.$suffix")
}

