// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import es.dipgra.grxfirma.android.model.CertificateDetail
import es.dipgra.grxfirma.android.model.CertificateSummary

/** Certificado abierto en la sesión: PKCS#12 importado o DNIe. Nunca lleva la clave. */
data class SessionIdentity(val certificate: CertificateSummary, val external: Boolean) {
    val id: String get() = certificate.id
}

/**
 * Lista de certificados abiertos, con las mismas reglas que el núcleo: abrir
 * uno ya abierto lo sustituye y solo hay un DNIe a la vez. Con un núcleo que
 * solo admite una identidad, abrir otra sustituye a la anterior.
 */
object SessionIdentities {
    fun add(current: List<SessionIdentity>, added: SessionIdentity, maxIdentities: Int): List<SessionIdentity> {
        if (maxIdentities <= 1) return listOf(added)
        return current.filterNot { it.id == added.id || (added.external && it.external) } + added
    }

    fun remove(current: List<SessionIdentity>, certificateId: String): List<SessionIdentity> =
        current.filterNot { it.id == certificateId }

    /** Tras cerrar uno se conserva el elegido; si era ese, se elige el último abierto. */
    fun selectionAfter(remaining: List<SessionIdentity>, selectedId: String?): SessionIdentity? =
        remaining.firstOrNull { it.id == selectedId } ?: remaining.lastOrNull()

    /** Aplica el filtro por NIF, organización o nombre y por tipo a la lista abierta. */
    fun filter(
        identities: List<SessionIdentity>,
        details: List<CertificateDetail>,
        query: String,
        kind: String,
    ): List<SessionIdentity> {
        if (query.isBlank() && kind.isBlank()) return identities
        val matching = CertificateFilter.apply(details, query, kind).map { it.id }.toSet()
        return identities.filter { it.id in matching }
    }
}
