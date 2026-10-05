// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.core

import io.github.aavidad.grxfirma.android.model.BatchItemInput
import io.github.aavidad.grxfirma.android.model.EniFileRequest
import io.github.aavidad.grxfirma.android.model.LoadedFile
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.util.Base64

class Wave4CodecTest {
    @Test fun `capabilities are read only when declared and never with remote exchange`() {
        val contract = JSONObject().put("services", JSONObject()
            .put("batch_cosign", true).put("external_signer_batch", true).put("batch_visible_seal", false)).toString()
        assertEquals(setOf(Wave4Capabilities.BATCH_COSIGN, Wave4Capabilities.EXTERNAL_BATCH), Wave4Codec.capabilities(contract))
        val remote = JSONObject().put("services", JSONObject().put("batch_cosign", true).put("remote_exchange", true)).toString()
        assertTrue(Wave4Codec.capabilities(remote).isEmpty())
        assertTrue(Wave4Codec.capabilities("no es JSON").isEmpty())
    }

    @Test fun `ENI file request carries documents, metadata and optional fields only when present`() {
        val docs = listOf(LoadedFile("a.xml", "application/xml", "<a/>".toByteArray()))
        val json = JSONObject(Wave4Codec.eniFileRequest(docs, "cert", EniFileRequest(listOf("L01180877"), "123", "E02")))
        assertEquals("cert", json.getString("certificate_id"))
        assertEquals("a.xml", json.getJSONArray("documents").getJSONObject(0).getString("name"))
        assertEquals("<a/>", String(Base64.getDecoder().decode(
            json.getJSONArray("documents").getJSONObject(0).getString("content_base64"))))
        assertEquals("E02", json.getString("state"))
        assertFalse(json.has("identifier"))
        assertFalse(json.has("interested"))
        val full = JSONObject(Wave4Codec.eniFileRequest(docs, "cert", EniFileRequest(listOf("L01180877"), "123", "E01",
            "ES_L01180877_2026_EXP1", "2026-10-01T00:00:00+02:00", listOf("Ana"))))
        assertEquals("ES_L01180877_2026_EXP1", full.getString("identifier"))
        assertEquals("2026-10-01T00:00:00+02:00", full.getString("opening_date"))
        assertEquals("Ana", full.getJSONArray("interested").getString(0))
    }

    @Test fun `ENI file response is either the XML or the issues per file`() {
        val xml = Base64.getEncoder().encodeToString("<exp/>".toByteArray())
        val ok = Wave4Codec.parseEniFile("""{"ok":true,"content_base64":"$xml","documents":2,"issues":[]}""")
        assertEquals("<exp/>", String(ok.bytes!!))
        assertEquals(2, ok.documents)
        val bad = Wave4Codec.parseEniFile(
            """{"ok":false,"documents":2,"issues":[{"field":"b.xml","key":"eni.validacion.structure","level":"error"}]}""")
        assertNull(bad.bytes)
        assertEquals("b.xml", bad.issues.single().field)
        assertEquals("eni.validacion.structure", bad.issues.single().key)
        assertThrows(CoreContractException::class.java) { Wave4Codec.parseEniFile("""{"ok":false,"issues":[]}""") }
        assertThrows(CoreContractException::class.java) { Wave4Codec.parseEniFile("""{"ok":true}""") }
    }

    @Test fun `batch items keep their own action and seal options`() {
        val pdf = LoadedFile("a.pdf", "application/pdf", byteArrayOf(1))
        val txt = LoadedFile("b.txt", "text/plain", byteArrayOf(2))
        val json = JSONObject(Wave4Codec.batchItemsRequest(listOf(
            BatchItemInput(pdf, "auto", "cosign", mapOf("visibleSeal" to "true")),
            BatchItemInput(txt, "cades", "cosign"),
        ), "cert", mapOf("profile" to "baseline")))
        val items = json.getJSONArray("items")
        assertEquals("", items.getJSONObject(0).getString("format"))
        assertEquals("cosign", items.getJSONObject(0).getString("action"))
        assertEquals("true", items.getJSONObject(0).getJSONObject("options").getString("visibleSeal"))
        assertFalse(items.getJSONObject(1).has("options"))
        assertEquals("baseline", json.getJSONObject("options").getString("profile"))
    }
}
