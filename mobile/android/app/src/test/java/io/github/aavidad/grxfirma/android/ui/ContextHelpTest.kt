// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Test

class ContextHelpTest {
    @Test fun `operation help follows the menu order`() {
        assertEquals(listOf(R.string.action_sign, R.string.action_cosign, R.string.action_countersign),
            ContextHelp.OPERATIONS.map { it.title })
        assertEquals(listOf(R.string.ayuda_operacion_firma, R.string.ayuda_operacion_cofirma,
            R.string.ayuda_operacion_contrafirma), ContextHelp.OPERATIONS.map { it.text })
    }

    @Test fun `every format in the menu has its own help`() {
        FormatPolicy.MENU_ORDER.forEach { assertNotNull(it, ContextHelp.formatText(it)) }
    }

    @Test fun `automatic shows only the general text and a format adds its own`() {
        assertEquals(listOf(ContextHelp.Section(null, R.string.ayuda_formato)), ContextHelp.format("auto"))
        assertEquals(listOf(ContextHelp.Section(null, R.string.ayuda_formato),
            ContextHelp.Section(R.string.format_pades, R.string.ayuda_formato_pades)), ContextHelp.format("pades"))
    }
}
