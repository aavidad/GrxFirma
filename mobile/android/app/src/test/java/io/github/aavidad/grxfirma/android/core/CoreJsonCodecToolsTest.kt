// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.core

import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.ProtectionRequest
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class CoreJsonCodecToolsTest {
    private val document = LoadedFile("nota.txt", "text/plain", "hola".encodeToByteArray())
    private fun b64(bytes: ByteArray) = Base64.getEncoder().encodeToString(bytes)

    @Test fun `tools are enabled only when the contract declares all of them`() {
        val services = JSONObject().put("process_batch", true).put("hash", true).put("protect", true)
            .put("unprotect", true).put("protect_sign", true).put("remote_exchange", false)
        assertTrue(CoreJsonCodec.toolsDeclared(JSONObject().put("services", services).toString()))
        services.put("protect_sign", false)
        assertFalse(CoreJsonCodec.toolsDeclared(JSONObject().put("services", services).toString()))
        services.put("protect_sign", true).put("remote_exchange", true)
        assertFalse(CoreJsonCodec.toolsDeclared(JSONObject().put("services", services).toString()))
    }

    @Test fun `hash request and response follow the contract`() {
        val request = JSONObject(CoreJsonCodec.hashRequest(document, "SHA-512", "base64"))
        assertEquals(setOf("content_base64", "algorithm", "format"), request.keys().asSequence().toSet())
        assertEquals("SHA-512", request.getString("algorithm"))
        val file = "abch".encodeToByteArray()
        val parsed = CoreJsonCodec.parseHash(JSONObject().put("algorithm", "SHA-256").put("format", "hex")
            .put("hash", "abch").put("output_base64", b64(file)).put("extension", "hexhash").toString())
        assertArrayEquals(file, parsed.bytes)
        assertEquals("hexhash", parsed.extension)
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseHash(JSONObject().put("algorithm", "SHA-256").put("format", "hex")
                .put("hash", "x").put("output_base64", b64(file)).put("extension", "../exe").toString())
        }
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseHash(JSONObject().put("algorithm", "SHA-256").put("format", "bin").put("hash", "x")
                .put("output_base64", b64(ByteArray(CoreJsonCodec.MAX_HASH_FILE_BYTES + 1))).put("extension", "hash").toString())
        }
        val check = JSONObject(CoreJsonCodec.hashCheckRequest(document, LoadedFile("nota.txt.hashb64", "text/plain", file)))
        assertEquals("nota.txt.hashb64", check.getString("hash_file_name"))
        val result = CoreJsonCodec.parseHashCheck(JSONObject().put("valid", false).put("algorithm", "SHA-1")
            .put("expected_hash", "a").put("actual_hash", "b").toString())
        assertFalse(result.valid)
    }

    @Test fun `protect request carries recipients, container and signing intent`() {
        val certificate = byteArrayOf(0x30, 0x01, 0x00)
        val plain = JSONObject(CoreJsonCodec.protectRequest(document,
            ProtectionRequest("authenvelopeddata", listOf(certificate), includeSessionCertificate = true)))
        assertEquals("authenvelopeddata", plain.getString("container"))
        assertTrue(plain.getBoolean("include_session_certificate"))
        assertFalse(plain.has("sign"))
        assertArrayEquals(certificate, Base64.getDecoder().decode(
            plain.getJSONArray("recipients").getJSONObject(0).getString("certificate_base64")))
        val signed = JSONObject(CoreJsonCodec.protectRequest(document,
            ProtectionRequest("signedandenvelopeddata", sign = true, certificateId = "id-1")))
        assertTrue(signed.getBoolean("sign"))
        assertEquals("id-1", signed.getString("certificate_id"))
        assertFalse(signed.has("recipients"))
        assertFalse(JSONObject(CoreJsonCodec.unprotectRequest(document)).has("container"))
    }

    @Test fun `file output names are sanitised and MIME types validated`() {
        val output = CoreJsonCodec.parseFileOutput(JSONObject().put("name", "../../nota.txt.enveloped")
            .put("mime_type", "application/pkcs7-mime").put("content_base64", b64(byteArrayOf(1, 2)))
            .put("container", "cms").toString(), "protección", "nota.txt")
        assertEquals("nota.txt.enveloped", output.displayName)
        assertEquals("application/pkcs7-mime", output.mimeType)
        val odd = CoreJsonCodec.parseFileOutput(JSONObject().put("name", "").put("mime_type", "bad mime")
            .put("content_base64", b64(byteArrayOf(1))).toString(), "desprotección", "fallback.bin")
        assertEquals("fallback.bin", odd.displayName)
        assertEquals("application/octet-stream", odd.mimeType)
    }

    @Test fun `batch keeps one result per input in order`() {
        val docs = listOf(document, LoadedFile("b.pdf", "application/pdf", byteArrayOf(1)))
        val request = JSONObject(CoreJsonCodec.batchRequest(docs, "auto", "cert", mapOf("profile" to "baseline")))
        val items = request.getJSONArray("items")
        assertEquals(2, items.length())
        assertEquals("", items.getJSONObject(0).getString("format"))
        assertEquals("sign", items.getJSONObject(1).getString("action"))
        assertFalse(request.has("session"))
        val response = JSONObject().put("ok", false).put("items", JSONArray()
            .put(JSONObject().put("index", 0).put("ok", true).put("format", "CAdES").put("algorithm", "SHA256withRSA")
                .put("signed_content_base64", b64(byteArrayOf(9))))
            .put(JSONObject().put("index", 1).put("ok", false).put("error", "x")))
        val results = CoreJsonCodec.parseBatch(response.toString(), docs)
        assertEquals("nota.p7s", results[0].output?.displayName)
        assertNull(results[1].output)
        assertEquals("b.pdf", results[1].sourceName)
        val shuffled = JSONObject().put("items", JSONArray()
            .put(JSONObject().put("index", 1).put("ok", false))
            .put(JSONObject().put("index", 0).put("ok", false)))
        assertThrows(CoreContractException::class.java) { CoreJsonCodec.parseBatch(shuffled.toString(), docs) }
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseBatch(JSONObject().put("items", JSONArray()).toString(), docs)
        }
    }

    @Test fun `unavailable core never runs tools and still erases secrets`() {
        val bridge = UnavailableCoreBridge(CoreReadiness(false, "verification_build", ""))
        assertFalse(bridge.toolsAvailable)
        val secret = "clave".toCharArray()
        assertThrows(CoreUnavailableException::class.java) { bridge.protect(document, ProtectionRequest("cms"), secret) }
        assertArrayEquals(CharArray(5), secret)
        val other = "clave".toCharArray()
        assertThrows(CoreUnavailableException::class.java) { bridge.unprotect(document, other) }
        assertArrayEquals(CharArray(5), other)
    }
}
