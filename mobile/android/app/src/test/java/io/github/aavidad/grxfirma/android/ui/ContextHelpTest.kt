// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class ContextHelpTest {
    @Test fun `operation help shows the general text and every option without a selection`() {
        assertEquals(listOf(null, R.string.action_sign, R.string.action_cosign, R.string.action_countersign),
            ContextHelp.OPERATIONS.map { it.title })
        assertEquals(listOf(R.string.ayuda_operacion, R.string.ayuda_operacion_firma, R.string.ayuda_operacion_cofirma,
            R.string.ayuda_operacion_contrafirma), ContextHelp.OPERATIONS.map { it.text })
        // Cofirma y contrafirma llevan «+»; la firma no lo necesita.
        assertEquals(listOf(null, null, R.string.ayuda_operacion_cofirma_mas, R.string.ayuda_operacion_contrafirma_mas),
            ContextHelp.OPERATIONS.map { it.more })
    }

    @Test fun `every format in the menu has its own help`() {
        (listOf("auto") + FormatPolicy.MENU_ORDER).forEach { assertNotNull(it, ContextHelp.formatText(it)) }
    }

    @Test fun `format help lists every option of the menu whatever is selected`() {
        val menu = listOf("auto", "pades", "cades", "xades")
        assertEquals(listOf(
            ContextHelp.Section(null, R.string.ayuda_formato),
            ContextHelp.Section(R.string.format_auto, R.string.ayuda_formato_automatico),
            ContextHelp.Section(R.string.format_pades, R.string.ayuda_formato_pades, R.string.ayuda_formato_pades_mas),
            ContextHelp.Section(R.string.format_cades, R.string.ayuda_formato_cades, R.string.ayuda_formato_cades_mas),
            ContextHelp.Section(R.string.format_xades, R.string.ayuda_formato_xades, R.string.ayuda_formato_xades_mas),
        ), ContextHelp.formats(menu))
    }

    @Test fun `a single text gets its plus only when it has an extended text`() {
        assertEquals(R.string.ayuda_sellado_tiempo_mas, ContextHelp.single(R.string.ayuda_sellado_tiempo).more)
        assertNull(ContextHelp.single(R.string.ayuda_tsa_servidor).more)
    }

    @Test fun `option name is bold whatever the language template`() {
        assertEquals("Firmar: firma." to (0 until 6), ContextHelp.compose("%1\$s: %2\$s", "Firmar", "firma."))
        assertEquals("Signer : signe." to (0 until 6), ContextHelp.compose("%1\$s : %2\$s", "Signer", "signe."))
        assertEquals("签名：说明" to (0 until 2), ContextHelp.compose("%1\$s：%2\$s", "签名", "说明"))
    }
}
