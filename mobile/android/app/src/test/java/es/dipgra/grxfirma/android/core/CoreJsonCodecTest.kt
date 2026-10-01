// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.core

import es.dipgra.grxfirma.android.model.LoadedFile
import java.util.Base64
import org.json.JSONObject
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class CoreJsonCodecTest {
    private val document = LoadedFile(
        displayName = "contrato.pdf",
        mimeType = "application/pdf",
        bytes = "%PDF-1.7".encodeToByteArray(),
    )

    @Test
    fun `sign request follows the mobilebind contract`() {
        val json = JSONObject(CoreJsonCodec.signRequest(document, "pades", "cert-1"))

        assertEquals("contrato.pdf", json.getString("name"))
        assertEquals("application/pdf", json.getString("mime_type"))
        assertEquals("pades", json.getString("format"))
        assertEquals("sign", json.getString("action"))
        assertEquals("cert-1", json.getString("certificate_id"))
        assertArrayEquals(document.bytes, Base64.getDecoder().decode(json.getString("content_base64")))
    }

    @Test
    fun `verification request includes the original only for detached signatures`() {
        val original = LoadedFile(
            displayName = "contrato.txt",
            mimeType = "text/plain",
            bytes = "contenido original".encodeToByteArray(),
        )

        val detached = JSONObject(CoreJsonCodec.verifyRequest(document, original))
        assertArrayEquals(
            original.bytes,
            Base64.getDecoder().decode(detached.getString("original_content_base64")),
        )

        val embedded = JSONObject(CoreJsonCodec.verifyRequest(document))
        assertFalse(embedded.has("original_content_base64"))
    }

    @Test
    fun `contract requires every security critical service`() {
        val valid = """
            {
              "contract_version": 1,
              "platform": "android",
              "services": {
                "sign": true,
                "verify": true,
                "select_certificate": true,
                "import_certificate": true
              }
            }
        """.trimIndent()

        CoreJsonCodec.parseContract(valid)

        val invalid = JSONObject(valid)
        invalid.getJSONObject("services").put("sign", false)
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseContract(invalid.toString())
        }
    }

    @Test
    fun `signed response is decoded and mapped to a safe output`() {
        val signed = "firma".encodeToByteArray()
        val response = JSONObject()
            .put("format", "PAdES")
            .put("algorithm", "SHA256withRSA")
            .put("certificate_id", "cert-1")
            .put("signed_content_base64", Base64.getEncoder().encodeToString(signed))
            .toString()

        val actual = CoreJsonCodec.parseSigned(response, "contrato.pdf")

        assertArrayEquals(signed, actual.bytes)
        assertEquals("contrato-firmado.pdf", actual.displayName)
        assertEquals("application/pdf", actual.mimeType)
    }

    @Test
    fun `verification response retains validity and bounded diagnostics`() {
        val response = """
            {
              "valid": true,
              "reason": "Cadena válida",
              "details": ["Cobertura total"],
              "signers": ["cert-1"],
              "format": "PAdES",
              "coverage": "full",
              "integrity_status": "valid",
              "certificate_status": "warning",
              "trust_status": "unknown",
              "revocation_mode": "embedded_evidence_only",
              "warnings": [],
              "errors": []
            }
        """.trimIndent()

        val actual = CoreJsonCodec.parseVerification(response)

        assertTrue(actual.valid)
        assertEquals("PAdES", actual.format)
        assertEquals("full", actual.coverage)
        assertEquals("valid", actual.integrityStatus)
        assertEquals("warning", actual.certificateStatus)
        assertEquals("unknown", actual.trustStatus)
        assertEquals("embedded_evidence_only", actual.revocationMode)
        assertEquals(listOf("cert-1"), actual.signers)
        assertFalse(actual.details.isEmpty())
    }

    @Test
    fun `unknown verification values fail closed in the Android model`() {
        val actual = CoreJsonCodec.parseVerification(
            """{"valid":true,"integrity_status":"invented","revocation_mode":"invented"}""",
        )

        assertEquals("unknown", actual.integrityStatus)
        assertEquals("unknown", actual.certificateStatus)
        assertEquals("unknown", actual.trustStatus)
        assertEquals("not_available", actual.revocationMode)
    }

    @Test
    fun `invalid base64 from core is rejected`() {
        val response = JSONObject()
            .put("format", "CAdES")
            .put("algorithm", "SHA256withRSA")
            .put("signed_content_base64", "%%%")
            .toString()

        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseSigned(response, "contrato.pdf")
        }
    }
}
