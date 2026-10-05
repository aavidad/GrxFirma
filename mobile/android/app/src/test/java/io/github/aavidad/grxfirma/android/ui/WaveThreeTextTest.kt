// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.model.EngineDiagnostics
import io.github.aavidad.grxfirma.android.model.RevocationCheck
import io.github.aavidad.grxfirma.android.model.TsaProbe
import io.github.aavidad.grxfirma.android.model.UpdateCheck
import io.github.aavidad.grxfirma.android.model.VeriFactuRecord
import io.github.aavidad.grxfirma.android.model.VeriFactuReport
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class WaveThreeTextTest {
    private fun UiText.flatten(): List<Any> = when (this) {
        is UiText.Lines -> lines.flatMap { it.flatten() }
        is UiText.Resource -> listOf<Any>(id) + arguments.flatMap { if (it is UiText) it.flatten() else listOf(it) }
        is UiText.Plural -> listOf(id, count)
        is UiText.Engine -> listOf(key)
        is UiText.DateTime -> listOf(iso)
        is UiText.Verification -> listOf(this)
    }

    @Test fun `expiry warns with words and counts days`() {
        val soon = MainViewModelWaveThreeTest.detail("a", "fisica", "", "", status = "expiring_soon", days = 12)
        val text = CertificateText.expiry(soon).flatten()
        assertTrue(R.string.cert_expires_soon in text)
        assertTrue(R.plurals.cert_days_left in text && 12 in text)
        assertTrue(CertificateText.warns(soon))
        val today = soon.copy(daysLeft = 0)
        assertTrue(R.string.cert_expires_today in CertificateText.expiry(today).flatten())
        val expired = soon.copy(status = "expired")
        assertTrue(R.string.cert_expired in CertificateText.expiry(expired).flatten())
        val valid = soon.copy(status = "valid")
        assertFalse(CertificateText.warns(valid))
        val lines = CertificateText.lines(valid.copy(nif = "", organization = "")).flatten()
        assertFalse(R.string.cert_nif in lines)
        assertFalse(R.string.cert_organization in lines)
    }

    @Test fun `revocation messages separate missing service from network failure`() {
        val base = RevocationCheck("unavailable", "", "", "", true, false)
        assertTrue(R.string.revocation_unavailable in RevocationText.lines(base).flatten())
        assertTrue(R.string.revocation_no_service in RevocationText.lines(base.copy(hasOcsp = false, status = "inconclusive")).flatten())
        assertTrue(R.string.revocation_valid in RevocationText.lines(base.copy(status = "valid")).flatten())
        val revoked = RevocationText.lines(base.copy(status = "revoked", revokedAt = "2026-09-01T10:00:00Z")).flatten()
        assertTrue(R.string.revocation_revoked in revoked && R.string.revocation_revoked_since in revoked)
        assertTrue(R.string.revocation_scope in revoked)
    }

    @Test fun `diagnostics keep only the TSA host and no personal data`() {
        assertEquals("tsa.example", DiagnosticsText.tsaHost("https://tsa.example:8443/private/path?token=abc"))
        assertEquals("", DiagnosticsText.tsaHost("no es una url"))
        val facts = AppFacts("0.0.116", "0123456789abcdef", "ab".repeat(32), "15", 35, "es-ES", "2026-10-05T10:00:00+02:00", "Europe/Madrid")
        val engine = EngineDiagnostics("0.0.116", 2, "android", "go1.26", "android/arm64", "2026-10-05T08:00:00Z", true)
        val text = DiagnosticsText.lines(facts, engine, "https://tsa.example/path?token=abc", null).flatten()
        assertTrue("tsa.example" in text)
        assertFalse(text.any { it.toString().contains("token") || it.toString().contains("/path") })
        assertTrue("0123456789ab" in text)
    }

    @Test fun `clock skew beyond two minutes asks to fix the date`() {
        val ok = TsaProbe("ok", true, "", "", 30, 200)
        assertTrue(R.string.diag_clock_ok in UiText.Lines(DiagnosticsText.tsaLines(ok)).flatten())
        assertTrue(R.string.diag_clock_ahead in UiText.Lines(DiagnosticsText.tsaLines(ok.copy(skewSeconds = 600))).flatten())
        assertTrue(R.string.diag_clock_behind in UiText.Lines(DiagnosticsText.tsaLines(ok.copy(skewSeconds = -600))).flatten())
        val http = UiText.Lines(DiagnosticsText.tsaLines(ok.copy(https = false, status = "timeout"))).flatten()
        assertTrue(R.string.diag_tsa_timeout in http && R.string.diag_tsa_http in http)
    }

    @Test fun `update messages map every status and error code`() {
        fun first(check: UpdateCheck) = (UpdateText.lines(check).lines.first() as UiText.Resource).id
        val base = UpdateCheck("error", "update_timeout", "0.0.115", "", "")
        assertEquals(R.string.update_error_timeout, first(base))
        assertEquals(R.string.update_error_rate_limited, first(base.copy(errorCode = "update_rate_limited")))
        assertEquals(R.string.update_error_network, first(base.copy(errorCode = "update_proxy_unavailable")))
        assertEquals(R.string.update_error_generic, first(base.copy(errorCode = "otro")))
        assertEquals(R.string.update_newer, first(base.copy(status = "newer")))
        assertEquals(R.string.update_current, first(base.copy(status = "current")))
        assertEquals(R.string.update_no_releases, first(base.copy(status = "no_releases")))
        assertTrue(R.string.update_install_hint in UpdateText.lines(base.copy(status = "newer")).flatten())
    }

    @Test fun `Veri*Factu export keeps the localized report and the engine result`() {
        val report = VeriFactuReport(true, 0, 0, listOf(VeriFactuRecord("a.xml", "RegistroAlta", "h", "h", "", true, true, emptyList())),
            reportJson = "{\"valid\":true,\"records\":[]}")
        val json = JSONObject(VeriFactuText.exportJson(report, "Informe traducido"))
        assertEquals("Informe traducido", json.getString("report"))
        assertTrue(json.getBoolean("valid"))
        assertTrue(json.getJSONObject("result").getBoolean("valid"))
    }
}
