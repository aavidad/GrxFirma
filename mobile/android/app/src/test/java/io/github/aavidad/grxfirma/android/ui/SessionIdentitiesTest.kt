// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreContractException
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.core.PlatformServices
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionIdentitiesTest {
    private fun identity(id: String, external: Boolean = false) =
        SessionIdentity(CertificateSummary(id, "Titular $id", "CA", id), external)

    @Test fun `opening another certificate keeps the ones already open`() {
        val list = SessionIdentities.add(SessionIdentities.add(emptyList(), identity("a"), 8), identity("b"), 8)
        assertEquals(listOf("a", "b"), list.map { it.id })
    }

    @Test fun `reopening the same certificate replaces it at the end`() {
        val list = listOf(identity("a"), identity("b"))
        assertEquals(listOf("b", "a"), SessionIdentities.add(list, identity("a"), 8).map { it.id })
    }

    @Test fun `a new DNIe replaces the previous one but not the files`() {
        val list = listOf(identity("a"), identity("dnie1", external = true))
        val after = SessionIdentities.add(list, identity("dnie2", external = true), 8)
        assertEquals(listOf("a", "dnie2"), after.map { it.id })
    }

    @Test fun `an old core keeps a single identity`() {
        val list = listOf(identity("a"))
        assertEquals(listOf("b"), SessionIdentities.add(list, identity("b"), 1).map { it.id })
    }

    @Test fun `closing the chosen certificate selects the last one still open`() {
        val remaining = SessionIdentities.remove(listOf(identity("a"), identity("b"), identity("c")), "c")
        assertEquals("b", SessionIdentities.selectionAfter(remaining, "c")?.id)
        assertEquals("a", SessionIdentities.selectionAfter(remaining, "a")?.id)
        assertNull(SessionIdentities.selectionAfter(emptyList(), "a"))
    }

    @Test fun `the existing filter narrows the open certificates`() {
        val details = listOf(
            MainViewModelWaveThreeTest.detail("a", "fisica", "12345678Z", "Diputación de Granada"),
            MainViewModelWaveThreeTest.detail("b", "sello", "Q1800000A", "Ayuntamiento"),
        )
        val identities = listOf(identity("a"), identity("b"))
        assertEquals(listOf("b"), SessionIdentities.filter(identities, details, "", "sello").map { it.id })
        assertEquals(listOf("a"), SessionIdentities.filter(identities, details, "granada", "").map { it.id })
        assertEquals(identities, SessionIdentities.filter(identities, details, "", ""))
        val state = MainUiState(CoreReadiness(true, "ready", ""), identities = identities, certificateDetails = details,
            certificateKindFilter = "fisica", maxIdentities = 8)
        assertTrue(state.showsIdentityList)
        assertTrue(state.canKeepSeveralIdentities)
        // Con dos certificados no hay buscador: se ven todos.
        assertEquals(listOf("a", "b"), state.filteredIdentities.map { it.id })
        val more = (1..3).map { MainViewModelWaveThreeTest.detail("x$it", "sello", "", "Otra") }
        val many = state.copy(identities = identities + more.map { identity(it.id) }, certificateDetails = details + more)
        assertEquals(listOf("a"), many.filteredIdentities.map { it.id })
        assertFalse(state.copy(identities = identities.take(1)).showsIdentityList)
    }

    @Test fun `reading a QR image depends on the core declaring it`() {
        val ready = MainUiState(CoreReadiness(true, "ready", ""), platformServices = setOf(PlatformServices.VERIFACTU_QR_IMAGE))
        assertTrue(ready.canReadQrImage)
        assertFalse(ready.copy(busy = true).canReadQrImage)
        assertFalse(ready.copy(platformServices = emptySet()).qrImageAvailable)
    }

    @Test fun `session full and image errors use Android texts`() {
        assertEquals(UiText.Resource(R.string.error_session_full), CoreContractException("session.full").toUserText())
        assertEquals(UiText.Resource(R.string.qr_error_not_found), CoreContractException("verifactu.qr_not_found").toUserText())
        assertEquals(UiText.Resource(R.string.qr_error_image), CoreContractException("verifactu.qr_image").toUserText())
        assertEquals(UiText.Resource(R.string.qr_error_timeout), CoreContractException("verifactu.qr_timeout").toUserText())
        assertEquals(UiText.Engine("verifactu.qr_url"), CoreContractException("verifactu.qr_url").toUserText())
    }
}
