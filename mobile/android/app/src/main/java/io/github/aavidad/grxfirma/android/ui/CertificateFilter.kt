// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.model.CertificateDetail
import java.text.Normalizer
import java.util.Locale

/** Filtro por NIF, organización o nombre y por tipo de certificado. */
object CertificateFilter {
    val KINDS = listOf("fisica", "representacion", "sello", "empleado_publico", "desconocido")

    fun apply(certificates: List<CertificateDetail>, query: String, kind: String): List<CertificateDetail> {
        val needle = normalize(query)
        return certificates.filter { certificate ->
            (kind.isBlank() || certificate.kind == kind) &&
                (needle.isEmpty() || listOf(certificate.nif, certificate.organization, certificate.subject)
                    .any { normalize(it).contains(needle) })
        }
    }

    /** Sin tildes, mayúsculas ni espacios: «12345678-z» encuentra «IDCES-12345678Z». */
    private fun normalize(value: String): String = Normalizer.normalize(value, Normalizer.Form.NFD)
        .replace(Regex("\\p{Mn}+"), "")
        .lowercase(Locale.ROOT)
        .filter { it.isLetterOrDigit() }
}
