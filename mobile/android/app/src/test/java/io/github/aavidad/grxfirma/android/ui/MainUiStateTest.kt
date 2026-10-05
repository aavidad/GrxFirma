// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreContractException
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MainUiStateTest {
    @Test
    fun `verification needs accredited certificate before success color`() {
        val base = UiText.Verification(
            valid = true,
            format = "PAdES",
            signerCount = 1,
            integrityStatus = "valid",
            certificateStatus = "valid",
            trustStatus = "valid",
            revocationMode = "embedded_evidence_only",
            warningCount = 0,
            errorCount = 0,
        )
        assertTrue(base.accredited())
        assertFalse(base.copy(certificateStatus = "unknown").accredited())
        assertFalse(base.copy(certificateStatus = "warning").accredited())
        assertFalse(base.copy(trustStatus = "unknown").accredited())
        assertFalse(base.copy(valid = false).accredited())
    }

    @Test
    fun `critical actions stay disabled without an operational backend`() {
        val state = MainUiState(
            backend = CoreReadiness(false, "verification_build", "No enlazado"),
        )

        assertFalse(state.canImportCertificate)
        assertFalse(state.canSign)
        assertFalse(state.canVerify)
        assertFalse(state.canForgetCertificate)
        assertTrue(state.canAcceptIncomingDocument)
    }

    @Test
    fun `pending save blocks replacement and incoming intents`() {
        val state = MainUiState(
            backend = CoreReadiness(true, "ready", "Disponible"),
            awaitingSave = true,
        )

        assertFalse(state.canReplaceSelection)
        assertFalse(state.canAcceptIncomingDocument)
        assertFalse(state.canSign)
        assertFalse(state.canVerify)
        assertTrue(state.canRetryPendingOutput)
        assertTrue(state.canDiscardPendingOutput)
    }

    @Test
    fun `only allowlisted certificate errors reach localized UI`() {
        val cases = mapOf(
            "No se pudo abrir el PKCS#12. Compruebe la contraseña; si usa un formato antiguo, reexpórtelo como PKCS#12 moderno (AES y SHA-256)." to
                R.string.error_pkcs12_password_or_legacy,
            "El certificado no está vigente. Use un certificado vigente; si ha caducado, renuévelo." to
                R.string.error_certificate_not_current,
            "El certificado o su clave no son aptos para firmar en Android. Use un certificado de firma con RSA de al menos 2048 bits o ECDSA de al menos 256 bits." to
                R.string.error_signing_identity_unsupported,
        )

        cases.forEach { (message, resource) ->
            assertEquals(UiText.Resource(resource), CoreContractException(message).toUserText())
        }
    }

    @Test
    fun `unexpected core details never reach UI`() {
        val secret = "contraseña=secreto-no-filtrar"

        val text = CoreContractException(secret).toUserText()

        assertEquals(UiText.Resource(R.string.error_core_operation), text)
        assertFalse(text.toString().contains(secret))
    }

    @Test
    fun `saved detail shows the name the provider really used`() {
        val detail = UiText.Lines(listOf(
            UiText.Plural(R.plurals.minutes, 2),
            UiText.Resource(R.string.result_unprotected_detail, listOf("doc.pdf")),
        ))
        val replaced = detail.replacingArgument("doc.pdf", "doc (1).pdf")
        assertEquals(UiText.Lines(listOf(
            UiText.Plural(R.plurals.minutes, 2),
            UiText.Resource(R.string.result_unprotected_detail, listOf("doc (1).pdf")),
        )), replaced)
    }
}
