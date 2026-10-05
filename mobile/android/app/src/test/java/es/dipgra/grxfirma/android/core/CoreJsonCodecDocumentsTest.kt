// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.core

import es.dipgra.grxfirma.android.model.EniRequest
import es.dipgra.grxfirma.android.model.LoadedFile
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class CoreJsonCodecDocumentsTest {
    private fun b64(bytes: ByteArray) = Base64.getEncoder().encodeToString(bytes)

    @Test fun `old contracts keep the three basic formats and no document services`() {
        val old = JSONObject().put("signing", JSONObject()).put("services", JSONObject()).toString()
        assertEquals(SignatureFormats.BASIC, CoreJsonCodec.signingFormats(old))
        assertTrue(CoreJsonCodec.documentServices(old).isEmpty())
        val current = JSONObject()
            .put("signing", JSONObject().put("formats", JSONArray(listOf("cades", "pades", "xades", "odf", "verifactu", "pkcs1"))))
            .put("services", JSONObject().put("verifactu_validate", true).put("eni_document", true).put("csv_legend", false))
            .toString()
        assertEquals(listOf("cades", "pades", "xades", "odf", "verifactu"), CoreJsonCodec.signingFormats(current))
        assertEquals(setOf(DocumentServices.VERIFACTU, DocumentServices.ENI_DOCUMENT), CoreJsonCodec.documentServices(current))
        val remote = JSONObject(current).apply { getJSONObject("services").put("remote_exchange", true) }.toString()
        assertTrue(CoreJsonCodec.documentServices(remote).isEmpty())
    }

    @Test fun `Veri*Factu report keeps records and closed issue keys`() {
        val request = JSONObject(CoreJsonCodec.veriFactuRequest(listOf(LoadedFile("a.xml", "application/xml", byteArrayOf(1)))))
        assertEquals("a.xml", request.getJSONArray("files").getJSONObject(0).getString("name"))
        val raw = JSONObject().put("valid", false).put("errors", 1).put("warnings", 1).put("records", JSONArray().put(
            JSONObject().put("file", "a.xml").put("type", "RegistroAlta").put("hash", "AA").put("calculated_hash", "BB")
                .put("previous_hash", "").put("signed", false).put("valid", false).put("issues", JSONArray()
                    .put(JSONObject().put("field", "Huella").put("key", "verifactu.hash").put("level", "error"))
                    .put(JSONObject().put("field", "Signature").put("key", "verifactu.unsigned").put("level", "warning"))),
        )).toString()
        val report = CoreJsonCodec.parseVeriFactu(raw)
        assertFalse(report.valid)
        assertEquals(1, report.errors)
        assertEquals("BB", report.records.single().calculatedHash)
        assertEquals(listOf("verifactu.hash", "verifactu.unsigned"), report.records.single().issues.map { it.key })
        assertThrows(CoreContractException::class.java) { CoreJsonCodec.parseVeriFactu("{}") }
    }

    @Test fun `ENI request carries only the filled metadata`() {
        val signature = LoadedFile("firma.csig", "application/pkcs7-signature", byteArrayOf(0x30))
        val json = JSONObject(CoreJsonCodec.eniDocumentRequest(signature, null,
            EniRequest(listOf("L01180877"), "administracion", "EE01", "TD10")))
        assertEquals(setOf("signature_base64", "organs", "origin", "state", "document_type"), json.keys().asSequence().toSet())
        val full = JSONObject(CoreJsonCodec.eniDocumentRequest(signature, LoadedFile("o.txt", "text/plain", byteArrayOf(2)),
            EniRequest(listOf("L01180877"), "ciudadano", "EE02", "TD99", "", "ES_L01180877_2026_ORIGEN", "2026-10-05T00:00:00+02:00", "TXT")))
        assertEquals("ES_L01180877_2026_ORIGEN", full.getString("source_identifier"))
        assertEquals(b64(byteArrayOf(2)), full.getString("original_base64"))
    }

    @Test fun `ENI responses are validated`() {
        val document = CoreJsonCodec.parseEniDocument(JSONObject().put("content_base64", b64("<x/>".encodeToByteArray()))
            .put("signature_type", "TF04").toString())
        assertEquals("TF04", document.signatureType)
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseEniDocument(JSONObject().put("content_base64", b64(byteArrayOf(1))).put("signature_type", "XX").toString())
        }
        val validation = CoreJsonCodec.parseEniValidation(JSONObject().put("valid", false).put("issues", JSONArray()
            .put(JSONObject().put("field", "documento").put("key", "eni.validacion.structure"))).toString())
        assertEquals("error", validation.issues.single().level)
        val catalogs = CoreJsonCodec.parseEniCatalogs(JSONObject().put("document_states", JSONArray(listOf("EE01", "malo")))
            .put("document_types", JSONArray()).toString())
        assertEquals(listOf("EE01"), catalogs.documentStates)
        assertEquals(SignatureFormats.DEFAULT_ENI_CATALOGS.documentTypes, catalogs.documentTypes)
        assertEquals("TD99", catalogs.documentTypes.last())
    }

    @Test fun `CSV legend response must be an HTTPS address`() {
        val legend = CoreJsonCodec.parseCsvLegend(JSONObject().put("url", "https://sede.xn--diputacin-d7a.es/c?csv=A")
            .put("text", "CSV: A").toString())
        assertEquals("CSV: A", legend.text)
        assertThrows(CoreContractException::class.java) {
            CoreJsonCodec.parseCsvLegend(JSONObject().put("url", "http://sede.example").put("text", "x").toString())
        }
        val request = JSONObject(CoreJsonCodec.csvLegendRequest("A", "sede.example", ""))
        assertFalse(request.has("csv_text"))
    }

    @Test fun `signed output keeps office types and names ASiC containers`() {
        assertEquals("odt" to "application/vnd.oasis.opendocument.text",
            CoreJsonCodec.signedFileType("ODF", "acta.odt", "application/vnd.oasis.opendocument.text"))
        assertEquals("docx" to "application/octet-stream", CoreJsonCodec.signedFileType("OOXML", "informe", "text/plain"))
        assertEquals("asics" to "application/vnd.etsi.asic-s+zip", CoreJsonCodec.signedFileType("ASiC-XAdES", "a.txt", "text/plain"))
        assertEquals("xml" to "application/xml", CoreJsonCodec.signedFileType("VeriFactu", "r.xml", "application/xml"))
        assertEquals("p7s" to "application/pkcs7-signature", CoreJsonCodec.signedFileType("CAdES", "a.txt", "text/plain"))
    }
}
