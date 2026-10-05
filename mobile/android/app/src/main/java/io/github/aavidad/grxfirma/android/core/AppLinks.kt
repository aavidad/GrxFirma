// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.core

import java.net.URI

/** Destinos oficiales que la app abre en el navegador; no son textos visibles. */
object AppLinks {
    const val RELEASES = "https://github.com/aavidad/GrxFirma/releases"
    /** Contacto de soporte; fuera de los textos traducibles. */
    const val CONTACT_EMAIL = "avidad@dipgra.es"
    private const val TAG_PREFIX = "/aavidad/GrxFirma/releases/tag/"
    /**
     * Etiqueta de la publicación: empieza por letra o número y solo admite
     * letras, números, punto, guion y guion bajo. Así quedan fuera «/», «%»
     * (cualquier «.» o «/» codificado) y los segmentos «.» y «..».
     */
    private val TAG = Regex("[A-Za-z0-9][A-Za-z0-9._-]{0,63}")

    /** Solo se abre una publicación del repositorio oficial por HTTPS. */
    fun isOfficialRelease(url: String): Boolean {
        if (url == RELEASES) return true
        val uri = try { URI(url) } catch (_: Exception) { return false }
        val path = uri.rawPath.orEmpty()
        if (uri.scheme != "https" || uri.rawAuthority != "github.com" || uri.host != "github.com" || uri.port != -1 ||
            uri.rawUserInfo != null || uri.rawQuery != null || uri.rawFragment != null || !path.startsWith(TAG_PREFIX)) {
            return false
        }
        val tag = path.substring(TAG_PREFIX.length)
        return TAG.matches(tag) && !tag.contains("..")
    }
}
