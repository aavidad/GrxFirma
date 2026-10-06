// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import org.junit.Assert.assertEquals
import org.junit.Test

class ProfileHelpTest {
    @Test fun `help explains every profile in menu order and ends with the network notice`() {
        assertEquals(
            listOf(R.string.profile_help_b, R.string.profile_help_t, R.string.profile_help_lt,
                R.string.profile_help_lta, R.string.profile_help_internet),
            ProfileHelp.PARAGRAPHS,
        )
        assertEquals(listOf("baseline", "t", "lt", "lta"),
            io.github.aavidad.grxfirma.android.settings.AppSettings.PROFILES)
    }

    @Test fun `paragraphs are separated by a blank line`() {
        val names = mapOf(R.string.profile_help_b to "B", R.string.profile_help_t to "T",
            R.string.profile_help_lt to "LT", R.string.profile_help_lta to "LTA",
            R.string.profile_help_internet to "Red")
        assertEquals("B\n\nT\n\nLT\n\nLTA\n\nRed", ProfileHelp.message { names.getValue(it) })
    }
}
