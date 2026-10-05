// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.nfc

import org.junit.Assert.*
import org.junit.Test

class DniePinHoldTest {
    @Test fun `single signatures never keep the PIN`() {
        val hold = DniePinHold()
        assertFalse(hold.keepPinAfterSuccess())
        hold.failed()
        assertTrue("fuera de una operación cada intento es independiente", hold.mayContactCard())
    }

    @Test fun `an operation keeps the PIN until the first failure and then stops using the card`() {
        val hold = DniePinHold()
        hold.begin()
        assertTrue(hold.keepPinAfterSuccess())
        assertTrue(hold.mayContactCard())
        hold.failed()
        assertFalse("un PIN erróneo no se repite en cada documento", hold.mayContactCard())
        hold.end()
        assertTrue(hold.mayContactCard())
        assertFalse(hold.keepPinAfterSuccess())
    }
}
