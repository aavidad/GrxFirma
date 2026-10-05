// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import org.junit.Assert.*
import org.junit.Test

class SigningOptionsTest {
    @Test fun `timestamp profiles require enabled TSA and a valid URL`() {
        for (profile in listOf("t", "lt", "lta")) {
            assertThrows(IllegalArgumentException::class.java) { SigningOptions.create(profile, false, "") }
            for (url in listOf("http://tsa.example/rfc3161", "https://tsa.example:443/time")) {
                assertEquals(url, SigningOptions.create(profile, true, url)["tsaURL"])
            }
        }
        assertFalse(SigningOptions.create("baseline", false, "ignored").containsKey("tsaURL"))
    }

    @Test fun `credentials fragments and non HTTP URLs cannot leave the UI`() {
        for (url in listOf("https://user:pass@tsa.example", "https://tsa.example/#", "https://tsa.example/#x",
            "file:///tmp/tsa", "//tsa.example", "https://tsa.example:0", "https://tsa.example:65536")) {
            assertThrows(IllegalArgumentException::class.java) { SigningOptions.create("t", true, url) }
        }
    }

    @Test fun `format limitations never silently lower the selected profile`() {
        assertFalse(SigningOptions.supported("pades", "countersign", "baseline"))
        assertFalse(SigningOptions.supported("pades", "sign", "lta"))
        assertFalse(SigningOptions.supported("xades", "cosign", "lt"))
        assertTrue(SigningOptions.supported("cades", "countersign", "lta"))
    }
}
