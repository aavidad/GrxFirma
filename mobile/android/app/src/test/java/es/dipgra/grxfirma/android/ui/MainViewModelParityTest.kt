// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import android.net.Uri
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreReadiness
import es.dipgra.grxfirma.android.core.UnavailableCoreBridge
import es.dipgra.grxfirma.android.files.DocumentRepository
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.model.VerificationSummary
import org.junit.Assert.*
import org.junit.Test

class MainViewModelParityTest {
    private val repository = object : DocumentRepository {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile = error("Unexpected I/O")
        override fun loadDocument(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun loadCertificate(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun write(uri: Uri, bytes: ByteArray): Unit = error("Unexpected I/O")
    }
    private val readiness = CoreReadiness(false, "verification_build", "")
    private fun model() = MainViewModel(repository, UnavailableCoreBridge(readiness))

    @Test fun `operation profile and TSA settings live in immutable state`() {
        val vm = model()
        val before = vm.state.value
        vm.updateSigningSettings("cosign", "lt", true, "http://tsa.example")
        assertEquals("sign", before.signatureAction)
        assertEquals("cosign", vm.state.value.signatureAction)
        assertEquals("lt", vm.state.value.signatureProfile)
        assertTrue(vm.state.value.tsaEnabled)
        assertEquals("http://tsa.example", vm.state.value.tsaUrl)
        vm.acceptCoSignSuggestion()
        assertFalse(vm.state.value.coSignSuggested)
    }

    @Test fun `missing input reports a resource and erases the supplied password`() {
        val vm = model()
        vm.sign("auto")
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_document_required)), vm.state.value.result)
        val password = charArrayOf('s', 'e', 'c', 'r', 'e', 't')
        vm.importCertificate(password)
        assertArrayEquals(CharArray(6), password)
        assertEquals(OperationResult.Error(UiText.Resource(R.string.no_certificate_file)), vm.state.value.result)
    }

    @Test fun `report picker blocks input changes and a report is never signing output`() {
        val report = VerificationSummary(false, "", emptyList(), listOf("id"), "CAdES", "partial",
            "valid", "unknown", "unknown", "embedded_evidence_only", emptyList(), emptyList(), reportJson = "{}")
        val state = MainUiState(readiness, verification = report)
        assertTrue(state.canExportReport)
        assertFalse(state.awaitingSave)
        val exporting = state.copy(awaitingReportSave = true)
        assertFalse(exporting.canReplaceSelection)
        assertFalse(exporting.canExportReport)
        assertFalse(exporting.canAcceptIncomingDocument)
        assertFalse(exporting.canDiscardPendingOutput)
        assertFalse(report.toUiText().accredited())
    }
}
