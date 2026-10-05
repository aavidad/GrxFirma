// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.model.SignerSummary
import io.github.aavidad.grxfirma.android.model.VerificationSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class VerificationCardTest {
    private val accredited = UiText.Verification(
        valid = true,
        reason = "firma PAdES válida",
        format = "PAdES",
        signerCount = 1,
        integrityStatus = "valid",
        certificateStatus = "valid",
        trustStatus = "valid",
        revocationMode = "embedded_evidence_only",
        warningCount = 0,
        errorCount = 0,
        coverage = "full",
        signerSummaries = listOf(SignerSummary("1", "CN=PRUEBA SINTETICA 2,O=GrxFirma QA,C=ES", "CN=CA", "ab")),
    )

    @Test
    fun `verdict follows integrity certificate and trust`() {
        assertEquals(Verdict.VALID, VerificationCard.verdict(accredited))
        // Caso real de la revisión: íntegra, pero sin anclas de confianza en el móvil.
        val untrusted = accredited.copy(certificateStatus = "unknown", trustStatus = "unknown")
        assertEquals(Verdict.INTACT_UNCHECKED, VerificationCard.verdict(untrusted))
        assertEquals(Verdict.INVALID, VerificationCard.verdict(accredited.copy(valid = false)))
        assertEquals(Verdict.INVALID, VerificationCard.verdict(accredited.copy(integrityStatus = "invalid")))
        assertEquals(Verdict.INVALID, VerificationCard.verdict(accredited.copy(certificateStatus = "invalid")))
        assertEquals(Verdict.INCOMPLETE, VerificationCard.verdict(untrusted.copy(integrityStatus = "unknown")))
        assertEquals(Verdict.REVOCATION_INCONCLUSIVE,
            VerificationCard.verdict(accredited.copy(valid = false, reason = "revocación no concluyente")))
    }

    @Test
    fun `title and color never contradict the verdict`() {
        assertEquals(R.string.verdict_valid, VerificationCard.title(Verdict.VALID))
        assertEquals(R.string.verdict_intact_unchecked, VerificationCard.title(Verdict.INTACT_UNCHECKED))
        assertEquals(R.string.verdict_invalid, VerificationCard.title(Verdict.INVALID))
        assertEquals(R.color.primary, VerificationCard.color(Verdict.VALID))
        assertEquals(R.color.status_warning, VerificationCard.color(Verdict.INTACT_UNCHECKED))
        assertEquals(R.color.error, VerificationCard.color(Verdict.INVALID))
        // «Conclusión: firma PAdES válida» no puede aparecer bajo un veredicto ámbar.
        assertFalse(VerificationCard.showsReason(Verdict.INTACT_UNCHECKED, "firma PAdES válida"))
        assertFalse(VerificationCard.showsReason(Verdict.VALID, "firma PAdES válida"))
        assertTrue(VerificationCard.showsReason(Verdict.INVALID, "firma alterada"))
        assertFalse(VerificationCard.showsReason(Verdict.INVALID, ""))
    }

    @Test
    fun `summary is short and in plain language`() {
        val untrusted = accredited.copy(certificateStatus = "unknown", trustStatus = "unknown")
        assertEquals(
            listOf(
                UiText.Resource(R.string.verification_signed_by, listOf("PRUEBA SINTETICA 2")),
                UiText.Resource(R.string.verification_integrity_ok),
                UiText.Resource(R.string.verification_certificate_unknown),
            ),
            VerificationCard.summary(untrusted),
        )
        val tampered = accredited.copy(valid = false, integrityStatus = "invalid", coverage = "partial")
        val lines = VerificationCard.summary(tampered)
        assertTrue(UiText.Resource(R.string.verification_integrity_bad) in lines)
        assertTrue(UiText.Resource(R.string.verification_partial) in lines)
        assertEquals(UiText.Resource(R.string.verification_certificate_ok), lines.last())
    }

    @Test
    fun `common name keeps the readable part of the subject`() {
        assertEquals("Ana Pérez", VerificationCard.commonName("CN=Ana Pérez,O=Diputación,C=ES"))
        assertEquals("Pérez, Ana", VerificationCard.commonName("C=ES,CN=Pérez\\, Ana"))
        assertEquals("O=Sin nombre", VerificationCard.commonName("O=Sin nombre"))
    }

    @Test
    fun `a new operation clears the previous result and verification`() {
        val summary = VerificationSummary(
            valid = true, reason = "", details = emptyList(), signers = emptyList(), format = "PAdES",
            coverage = "full", integrityStatus = "valid", certificateStatus = "valid", trustStatus = "valid",
            revocationMode = "online", warnings = emptyList(), errors = emptyList(), reportJson = "{}", reportHtml = "<html></html>",
        )
        val afterVerify = MainUiState(
            CoreReadiness(true, "ready", ""),
            verification = summary,
            verifiedDocumentName = "contrato.pdf",
            postSignVerificationFailed = true,
            result = OperationResult.Success(UiText.Resource(R.string.verification_result_title), summary.toUiText()),
        )
        assertTrue(afterVerify.canExportReport)

        val cleared = afterVerify.clearedResult()
        assertEquals(OperationResult.Idle, cleared.result)
        assertNull(cleared.verification)
        assertEquals("", cleared.verifiedDocumentName)
        assertFalse(cleared.postSignVerificationFailed)
        assertFalse(cleared.canExportReport)

        // Guardar preferencias o un lote deja solo su propio resultado.
        val saved = OperationResult.Success(UiText.Resource(R.string.preferences_saved))
        val afterPreferences = afterVerify.clearedResult(saved)
        assertEquals(saved, afterPreferences.result)
        assertNull(afterPreferences.verification)
    }
}
