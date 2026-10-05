// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.core

import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.ProtectionRequest
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class CoreJsonCodecIdentitiesTest {
    @Test fun `max identities defaults to one and is bounded`() {
        assertEquals(1, CoreJsonCodec.maxIdentities("""{"contract_version":2}"""))
        assertEquals(8, CoreJsonCodec.maxIdentities("""{"identity_store":{"max_identities":8}}"""))
        assertEquals(16, CoreJsonCodec.maxIdentities("""{"identity_store":{"max_identities":900}}"""))
        assertEquals(1, CoreJsonCodec.maxIdentities("""{"identity_store":{"max_identities":0}}"""))
    }

    @Test fun `removal response must confirm the removal`() {
        assertEquals(2, CoreJsonCodec.parseRemainingIdentities("""{"removed":true,"remaining":2}"""))
        assertThrows(CoreContractException::class.java) { CoreJsonCodec.parseRemainingIdentities("""{"removed":false}""") }
    }

    @Test fun `new services are part of the platform list`() {
        val services = CoreJsonCodec.platformServices(
            """{"services":{"verifactu_qr_image":true,"session_identities":true,"remote_exchange":false}}""")
        assertTrue(PlatformServices.VERIFACTU_QR_IMAGE in services)
        assertTrue(PlatformServices.SESSION_IDENTITIES in services)
    }

    @Test fun `protection names the chosen certificate also for encrypt for me`() {
        val document = LoadedFile("a.txt", "text/plain", byteArrayOf(1))
        val forMe = JSONObject(CoreJsonCodec.protectRequest(document,
            ProtectionRequest("cms", emptyList(), includeSessionCertificate = true, sign = false, certificateId = "id-2")))
        assertEquals("id-2", forMe.getString("certificate_id"))
        assertFalse(forMe.has("sign"))
        val anonymous = JSONObject(CoreJsonCodec.protectRequest(document,
            ProtectionRequest("cms-encrypted", emptyList(), includeSessionCertificate = false, sign = false, certificateId = "")))
        assertFalse(anonymous.has("certificate_id"))
    }
}
