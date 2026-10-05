// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.core

import io.github.aavidad.grxfirma.android.model.LoadedFile
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
              "contract_version": 2,
              "platform": "android",
              "signing": {"actions":["sign","cosign","countersign"], "profiles_by_format":{"CAdES":["baseline","t","lt","lta"]}},
              "services": {
                "sign": true,
                "verify": true,
                "select_certificate": true,
                "import_certificate": true,
                "external_signer": true
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
        assertEquals("contrato.pdf", actual.displayName)
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
    fun `verification shows localized evidence and keeps raw keys in the technical json`() {
        val response = """{"valid":true,"details":["formato_detectado=PAdES"],"details_text":["Detected format: PAdES"]}"""
        val actual = CoreJsonCodec.parseVerification(response)
        assertEquals(listOf("Detected format: PAdES"), actual.details)
        assertTrue(actual.reportJson.contains("formato_detectado=PAdES"))
        assertFalse(actual.reportJson.contains("details_text"))
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
    @Test
    fun `co and countersign actions and TSA profiles reach the facade`() {
        for (action in listOf("cosign", "countersign")) {
            val request = JSONObject(CoreJsonCodec.signRequest(document, "cades", "id",
                mapOf("profile" to "lt", "tsaURL" to "http://tsa.example"), action))
            assertEquals(action, request.getString("action"))
            assertEquals("lt", request.getJSONObject("options").getString("profile"))
        }
    }

    @Test
    fun `signer summaries and full report survive parsing`() {
        val raw = """{"valid":false,"coverage":"partial","details":["evidence"],
            "signer_summaries":[{"id":"id","subject":"S","issuer":"I","fingerprint":"F"}],
            "warnings":["warning"],"errors":["error"]}"""
        val report = CoreJsonCodec.parseVerification(raw)
        assertEquals("S", report.signerSummaries.single().subject)
        assertEquals("partial", report.coverage)
        assertEquals("evidence", report.details.single())
        assertEquals("I", JSONObject(report.reportJson).getJSONArray("signer_summaries").getJSONObject(0).getString("issuer"))
    }

    @Test
    fun `an old AAR cannot enable the new flows`() {
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseContract("""{"contract_version":1,"platform":"android"}""")
        }
    }
}
