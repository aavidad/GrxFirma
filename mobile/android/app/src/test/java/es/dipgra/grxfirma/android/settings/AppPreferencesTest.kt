// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.settings

import es.dipgra.grxfirma.android.core.AppLinks
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppPreferencesTest {
    @Test fun `tampered values fall back to safe defaults`() {
        val clean = AppSettings(defaultFormat = "pkcs1", defaultProfile = "ltv", outputName = "../x", theme = "rojo",
            tsaUrl = "https://tsa.example\n" + "a".repeat(5000)).sanitized()
        assertEquals("auto", clean.defaultFormat)
        assertEquals("baseline", clean.defaultProfile)
        assertEquals(OutputNames.SUFFIX, clean.outputName)
        assertEquals(AppSettings.THEME_SYSTEM, clean.theme)
        assertTrue(clean.tsaUrl.length <= AppSettings.MAX_TSA_URL)
        assertFalse(clean.tsaUrl.contains('\n'))
    }

    @Test fun `seal language is one of the catalogue and only goes with a visible seal`() {
        assertEquals("", AppSettings(sealLanguage = "klingon").sanitized().sealLanguage)
        assertEquals("va", AppSettings(sealLanguage = "va").sanitized().sealLanguage)
        val seal = mapOf("visibleSeal" to "true")
        assertEquals("es", AppSettings(sealLanguage = "es").withSealLanguage(seal)[AppSettings.SEAL_LANGUAGE_OPTION])
        assertEquals(seal, AppSettings().withSealLanguage(seal))
        assertEquals(emptyMap<String, String>(), AppSettings(sealLanguage = "es").withSealLanguage(emptyMap()))
    }

    @Test fun `output name follows the chosen policy`() {
        val suffix = "%1\$s-firmado.%2\$s"
        val desktop = "%1\$s_firmado.%2\$s"
        assertEquals("contrato-firmado.pdf", OutputNames.name(OutputNames.SUFFIX, "contrato", "pdf", suffix, desktop))
        assertEquals("contrato_firmado.pdf", OutputNames.name(OutputNames.DESKTOP, "contrato", "pdf", suffix, desktop))
        assertEquals("contrato-firmado.pdf", OutputNames.name(OutputNames.SUFFIX, "contrato-firmado", "pdf", suffix, desktop))
        assertEquals("contrato_firmado.pdf", OutputNames.name(OutputNames.DESKTOP, "contrato_firmado", "pdf", suffix, desktop))
        assertEquals("contrato.p7s", OutputNames.name(OutputNames.ORIGINAL, "contrato", "p7s", suffix, desktop))
        assertEquals("contrato-firmado.pdf", OutputNames.name("desconocida", "contrato", "pdf", suffix, desktop))
    }

    @Test fun `in-memory store resets to defaults`() {
        val store = InMemorySettingsStore(AppSettings(theme = AppSettings.THEME_DARK))
        store.reset()
        assertEquals(AppSettings(), store.load())
        assertEquals(1, store.resets)
    }

    @Test fun `only official GitHub releases can be opened`() {
        assertTrue(AppLinks.isOfficialRelease(AppLinks.RELEASES))
        assertTrue(AppLinks.isOfficialRelease("https://github.com/aavidad/GrxFirma/releases/tag/v0.0.120"))
        assertTrue(AppLinks.isOfficialRelease("https://github.com/aavidad/GrxFirma/releases/tag/v2.0.1-rc_1"))
        listOf(
            "http://github.com/aavidad/GrxFirma/releases/tag/v1",
            "https://github.com.evil.test/aavidad/GrxFirma/releases/tag/v1",
            "https://github.com/otro/GrxFirma/releases/tag/v1",
            "https://github.com/aavidad/GrxFirma/releases/tag/",
            "https://github.com/aavidad/GrxFirma/releases/tag/v1?x=1",
            "https://github.com/aavidad/GrxFirma/releases/tag/v1#f",
            "https://user@github.com/aavidad/GrxFirma/releases/tag/v1",
            "https://github.com:444/aavidad/GrxFirma/releases/tag/v1",
            "https://github.com/aavidad/GrxFirma/releases/tag/../../evil",
            "https://github.com/aavidad/GrxFirma/releases/tag/%2e%2e/%2e%2e/evil",
            "https://github.com/aavidad/GrxFirma/releases/tag/%2E%2E",
            "https://github.com/aavidad/GrxFirma/releases/tag/v1%2fx",
            "https://github.com/aavidad/GrxFirma/releases/tag/v1/../../x",
            "https://github.com/aavidad/GrxFirma/releases/tag/v1/",
            "https://github.com/aavidad/GrxFirma/releases/tag/.",
            "https://GitHub.com/aavidad/GrxFirma/releases/tag/v1",
        ).forEach { assertFalse(it, AppLinks.isOfficialRelease(it)) }
    }
}
