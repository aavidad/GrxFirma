// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import es.dipgra.grxfirma.android.R
import org.junit.Assert.assertEquals
import org.junit.Test

class BatchSavedTextTest {
    @Test fun `names the chosen folder without hidden characters`() {
        assertEquals(UiText.Plural(R.plurals.batch_saved_count_named, 2, listOf("Firmas")),
            batchSavedText(2, "‮Fir​mas\n"))
    }

    @Test fun `falls back to the generic text when the name is unknown`() {
        assertEquals(UiText.Plural(R.plurals.batch_saved_count, 1), batchSavedText(1, null))
        assertEquals(UiText.Plural(R.plurals.batch_saved_count, 3), batchSavedText(3, " ‎ "))
    }
}
