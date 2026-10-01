// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.core

import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Test

class UnavailableCoreBridgeTest {
    @Test
    fun `verification bridge never reports cryptographic success`() {
        val bridge = UnavailableCoreBridge(
            CoreReadiness(false, "verification_build", "Backend no enlazado"),
        )

        assertFalse(bridge.readiness.available)
        assertThrows(CoreUnavailableException::class.java) {
            bridge.selectCertificate()
        }
    }
}
