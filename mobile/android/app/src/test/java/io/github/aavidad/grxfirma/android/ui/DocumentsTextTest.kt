// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreContractException
import io.github.aavidad.grxfirma.android.model.EngineIssue
import io.github.aavidad.grxfirma.android.model.VeriFactuRecord
import io.github.aavidad.grxfirma.android.model.VeriFactuReport
import java.time.ZoneId
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DocumentsTextTest {
    @Test fun `DIR3 codes are normalised and validated`() {
        val organs = EniForm.organs(" l01180877, A12345678;l01180877 ")
        assertEquals(listOf("L01180877", "A12345678"), organs)
        assertTrue(EniForm.organsValid(organs))
        assertFalse(EniForm.organsValid(EniForm.organs("granada")))
        assertFalse(EniForm.organsValid(emptyList()))
        assertFalse(EniForm.organsValid(EniForm.organs((1..17).joinToString(",") { "L%08d".format(it) })))
    }

    @Test fun `capture date is the chosen day at local midnight in RFC 3339`() {
        val utcMidnight = 1_791_158_400_000L // 2026-10-05T00:00:00Z
        assertEquals("2026-10-05T00:00:00+02:00", EniForm.captureDate(utcMidnight, ZoneId.of("Europe/Madrid")))
        assertEquals("2026-10-05T00:00:00Z", EniForm.captureDate(utcMidnight, ZoneId.of("UTC")))
        assertEquals("", EniForm.captureDate(null))
        assertEquals("firma.eni.xml", EniForm.outputName("firma.pdf"))
    }

    @Test fun `Veri*Factu report lists each record with engine keys`() {
        val report = VeriFactuReport(false, 1, 1, listOf(VeriFactuRecord("a.xml", "RegistroAlta", "AA", "BB", "", false, false,
            listOf(EngineIssue("Huella", "verifactu.hash", "error"), EngineIssue("Signature", "verifactu.unsigned", "warning")))))
        val lines = VeriFactuText.lines(report).lines
        assertEquals(UiText.Plural(R.plurals.verifactu_record_count, 1), lines[0])
        assertTrue(lines.contains(UiText.Resource(R.string.issue_line, listOf("Huella", UiText.Engine("verifactu.hash")))))
        assertTrue(lines.contains(UiText.Resource(R.string.issue_warning_line, listOf("Signature", UiText.Engine("verifactu.unsigned")))))
    }

    @Test fun `only closed engine keys reach the screen`() {
        assertTrue(EngineKeys.isClosed("verifactu.root"))
        assertTrue(EngineKeys.isClosed("csv.error.url_invalid"))
        assertFalse(EngineKeys.isClosed("verifactu.root: detalle del motor"))
        assertFalse(EngineKeys.isClosed("panic: runtime error"))
        assertEquals(UiText.Engine("eni.validacion.dir3"), CoreContractException("eni.validacion.dir3").toUserText())
        assertEquals(UiText.Resource(R.string.eni_error_unsigned_pdf), CoreContractException("eni.error.unsigned_pdf").toUserText())
        assertEquals(UiText.Resource(R.string.error_format_requires_rsa),
            CoreContractException("El formato elegido solo admite certificados con clave RSA.").toUserText())
        assertEquals(UiText.Resource(R.string.error_core_operation), CoreContractException("otro").toUserText())
        assertEquals("url", EngineKeys.csvField("csv.error.url_missing"))
    }
}
