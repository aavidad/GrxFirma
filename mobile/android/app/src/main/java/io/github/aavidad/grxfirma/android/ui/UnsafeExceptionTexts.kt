// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context

/** Solo claves propias de Android; los textos de cada caso pertenecen al núcleo. */
object UnsafeExceptionTexts {
    val keys = setOf("seccion", "introduccion", "activar", "desactivar", "activar_continuar", "cancelar",
        "mas_informacion", "cerrar", "actualizar", "cargando", "guardando", "no_disponible", "reenviar",
        "activada", "desactivada", "fijada_activa", "fijada_inactiva", "directiva_ilegible", "version",
        "aceptada_fecha", "ver_bloqueo", "version_no_disponible", "aceptacion_casilla",
        "aceptar_titulo", "verificacion_bloqueada", "revisar_aviso", "activada_resultado", "desactivada_resultado")
        .map { "android.inseguro.$it" }.toSet()
    fun text(context: Context, suffix: String) = EngineText.resolve(context, "android.inseguro.$suffix")
}

