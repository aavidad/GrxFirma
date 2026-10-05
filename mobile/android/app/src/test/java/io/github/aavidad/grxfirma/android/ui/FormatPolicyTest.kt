// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class FormatPolicyTest {
    @Test fun `automatic format mirrors the mobile engine rules`() {
        val cases = listOf(
            Triple("a.pdf", "application/octet-stream", "pades"),
            Triple("documento", "application/pdf", "pades"),
            Triple("a.docx", "application/octet-stream", "ooxml"),
            Triple("hoja", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "ooxml"),
            Triple("a.odt", "application/octet-stream", "odf"),
            Triple("texto", "application/vnd.oasis.opendocument.text", "odf"),
            Triple("a.asics", "application/octet-stream", "asic-xades"),
            Triple("a.dsig", "application/octet-stream", "xmldsig"),
            Triple("factura.facturae.xml", "application/xml", "facturae"),
            Triple("datos.xml", "application/xml", "xades"),
            Triple("datos.txt", "text/plain", "cades"),
        )
        for ((name, mime, expected) in cases) assertEquals(name, expected, FormatPolicy.detect(name, mime))
    }

    @Test fun `FacturaE is recognised by its root element even with a prefix and prolog`() {
        val xml = "\uFEFF<?xml version=\"1.0\"?><!-- factura --><fe:Facturae xmlns:fe=\"http://www.facturae.es/Facturae/2014/v3.2.1/Facturae\"/>"
        assertTrue(FormatPolicy.isFacturaE(xml.encodeToByteArray()))
        assertEquals("facturae", FormatPolicy.detect("recibida.xml", "text/xml", xml.encodeToByteArray()))
        assertFalse(FormatPolicy.isFacturaE("<Invoice/>".encodeToByteArray()))
        assertFalse(FormatPolicy.isFacturaE(null))
    }

    @Test fun `actions and profiles follow the contract table`() {
        assertTrue(FormatPolicy.supported("odf", "cosign", "baseline"))
        assertFalse(FormatPolicy.supported("odf", "countersign", "baseline"))
        assertFalse(FormatPolicy.supported("xmldsig", "sign", "t"))
        assertFalse(FormatPolicy.supported("facturae", "cosign", "baseline"))
        assertFalse(FormatPolicy.supported("verifactu", "sign", "t"))
        assertTrue(FormatPolicy.supported("pades", "sign", "lt"))
        assertFalse(FormatPolicy.supported("pkcs1", "sign", "baseline"))
        assertTrue(FormatPolicy.acceptsTimestamp("cades"))
        assertFalse(FormatPolicy.acceptsTimestamp("asic-xades"))
        assertTrue(FormatPolicy.requiresRsa("ooxml"))
        assertFalse(FormatPolicy.requiresRsa("pades"))
        assertTrue(SigningOptions.supported("verifactu", "sign", "baseline"))
    }
}
