// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.core

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class CoreJsonCodecWaveThreeTest {
    @Test fun `platform services come only from the contract`() {
        val contract = """{"services":{"certificate_details":true,"update_check":true,"tsa_probe":false,"otro":true}}"""
        assertEquals(setOf("certificate_details", "update_check"), CoreJsonCodec.platformServices(contract))
    }

    @Test fun `certificate details close unknown values`() {
        val raw = """{"expiring_soon_days":30,"certificates":[{"certificate_id":"ab","subject":"Ana","issuer":"CA",
            "nif":"IDCES-1","organization":"Org","kind":"raro","key_type":"DSA","key_bits":2048,"not_before":"2026-01-01T00:00:00Z",
            "not_after":"2027-01-01T00:00:00Z","days_left":10,"status":"expiring_soon","external":true,"has_ocsp":true}]}"""
        val details = CoreJsonCodec.parseCertificateDetails(raw)
        val first = details.certificates.single()
        assertEquals("desconocido", first.kind)
        assertEquals("", first.keyType)
        assertEquals("expiring_soon", first.status)
        assertTrue(first.external && first.hasOcsp && !first.hasCrl)
    }

    @Test fun `revocation, TSA and update statuses are closed`() {
        assertEquals("unavailable", CoreJsonCodec.parseRevocation("""{"status":"hackeado","method":"LDAP"}""").status)
        assertEquals("", CoreJsonCodec.parseRevocation("""{"status":"valid","method":"LDAP"}""").method)
        assertEquals("bad_response", CoreJsonCodec.parseTsaProbe("""{"status":"???"}""").status)
        val update = CoreJsonCodec.parseUpdateCheck("""{"status":"newer","latest":"v9","url":"https://evil.test/x"}""")
        assertEquals("newer", update.status)
        assertEquals("", update.url)
    }

    @Test fun `QR requires HTTPS and the four fields`() {
        val ok = """{"url":"https://www2.agenciatributaria.gob.es/x","nif":"89890001K","numserie":"A","fecha":"01-01-2025","importe":"1"}"""
        assertEquals("89890001K", CoreJsonCodec.parseVeriFactuQr(ok).nif)
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseVeriFactuQr(ok.replace("https://", "http://"))
        }
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseVeriFactuQr(JSONObject(ok).apply { remove("importe") }.toString())
        }
    }

    @Test fun `AEAT response is shown indented and bounded`() {
        val text = CoreJsonCodec.parseVeriFactuQuery("""{"response":{"estado":"Correcto"}}""")
        assertTrue(text.contains("\"estado\": \"Correcto\""))
        val large = CoreJsonCodec.parseVeriFactuQuery("""{"response":{"x":"${"a".repeat(40000)}"}}""")
        assertTrue(large.length <= 16 * 1024)
        assertThrows(CoreContractException::class.java) { CoreJsonCodec.parseVeriFactuQuery("""{"otro":1}""") }
        assertFalse(CoreJsonCodec.parseVeriFactuQuery("""{"response":"a\u0007b"}""").contains('\u0007'))
    }
}
