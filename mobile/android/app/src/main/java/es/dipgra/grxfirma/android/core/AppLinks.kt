// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.core

import java.net.URI

/** Destinos oficiales que la app abre en el navegador; no son textos visibles. */
object AppLinks {
    const val RELEASES = "https://github.com/aavidad/GrxFirma/releases"
    /** Contacto de soporte; fuera de los textos traducibles. */
    const val CONTACT_EMAIL = "avidad@dipgra.es"
    private const val TAG_PREFIX = "/aavidad/GrxFirma/releases/tag/"

    /** Solo se abre una publicación del repositorio oficial por HTTPS. */
    fun isOfficialRelease(url: String): Boolean {
        if (url == RELEASES) return true
        val uri = try { URI(url) } catch (_: Exception) { return false }
        return uri.scheme == "https" && uri.host == "github.com" && uri.port == -1 && uri.rawUserInfo == null &&
            uri.rawQuery == null && uri.rawFragment == null && uri.rawPath.orEmpty().startsWith(TAG_PREFIX) &&
            uri.rawPath.length > TAG_PREFIX.length && !uri.rawPath.contains("..")
    }
}
