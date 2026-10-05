// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.net.Uri
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreBridge
import io.github.aavidad.grxfirma.android.core.CoreContractException
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.core.DocumentServices
import io.github.aavidad.grxfirma.android.core.SignatureFormats
import io.github.aavidad.grxfirma.android.files.ContentRepository
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.CsvLegend
import io.github.aavidad.grxfirma.android.model.EngineIssue
import io.github.aavidad.grxfirma.android.model.EniDocument
import io.github.aavidad.grxfirma.android.model.EniRequest
import io.github.aavidad.grxfirma.android.model.EniValidation
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
import io.github.aavidad.grxfirma.android.model.VeriFactuRecord
import io.github.aavidad.grxfirma.android.model.VeriFactuReport
import io.github.aavidad.grxfirma.android.model.VerificationSummary
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(AndroidJUnit4::class)
class MainViewModelDocumentsTest {
    @After fun resetDispatcher() = Dispatchers.resetMain()

    @Test fun veriFactuReportsEachRecordAndErasesTheInput() = runTest {
        val (vm, repository, core) = ready()
        vm.selectVeriFactuRecords(listOf(RECORD_URI, OTHER_URI))
        assertEquals(2, vm.state.value.verifactuRecords.size)
        vm.checkVeriFactu()
        advanceUntilIdle()
        assertEquals(2, core.lastRecords)
        val error = vm.state.value.result as OperationResult.Error
        val lines = (error.detail as UiText.Lines).lines
        assertEquals(UiText.Resource(R.string.verifactu_result_invalid), lines[0])
        repository.loaded.forEach { assertArrayEquals(ByteArray(it.size), it) }
        vm.selectVeriFactuRecords((0..64).map { Uri.parse("content://test/r$it") })
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_verifactu_too_many)), vm.state.value.result)
    }

    @Test fun eniDocumentIsSavedAsAToolResult() = runTest {
        val (vm, _, core) = ready()
        vm.createEni(EniRequest(listOf("granada"), "administracion", "EE01", "TD99"))
        assertEquals(OperationResult.Error(UiText.Engine("eni.validacion.dir3")), vm.state.value.result)
        vm.updateEniCaptureDate(1_791_158_400_000L)
        vm.createEni(EniRequest(listOf("L01180877"), "administracion", "EE01", "TD99",
            captureDate = EniForm.captureDate(vm.state.value.eniCaptureDate)))
        advanceUntilIdle()
        assertTrue(core.lastEni!!.captureDate.startsWith("2026-10-05T00:00:00"))
        assertTrue(vm.state.value.awaitingSave)
        assertEquals(PendingKind.TOOL, vm.state.value.pendingKind)
        assertEquals("id", vm.state.value.certificate?.id)
    }

    @Test fun eniEngineErrorsAreShownWithTheirClosedKey() = runTest {
        val (vm, _, core) = ready()
        core.eniFailure = "eni.error.explicit_cades"
        vm.createEni(EniRequest(listOf("L01180877"), "ciudadano", "EE01", "TD99"))
        advanceUntilIdle()
        assertEquals(OperationResult.Error(UiText.Resource(R.string.eni_error_explicit_cades)), vm.state.value.result)
        vm.validateEni(ENI_URI)
        advanceUntilIdle()
        val error = vm.state.value.result as OperationResult.Error
        assertTrue((error.detail as UiText.Lines).lines.any { it is UiText.Resource && it.id == R.string.issue_line })
    }

    private fun TestScope.ready(): Triple<MainViewModel, FakeRepository, FakeCore> {
        Dispatchers.setMain(StandardTestDispatcher(testScheduler))
        val repository = FakeRepository()
        val core = FakeCore()
        val vm = MainViewModel(repository, core, StandardTestDispatcher(testScheduler))
        vm.selectDocument(DOCUMENT_URI)
        advanceUntilIdle()
        vm.selectCertificateFile(CERTIFICATE_URI)
        vm.importCertificate(charArrayOf('p'))
        advanceUntilIdle()
        assertTrue(vm.state.value.verifactuAvailable && vm.state.value.eniDocumentAvailable)
        return Triple(vm, repository, core)
    }

    private class FakeRepository : ContentRepository(ApplicationProvider.getApplicationContext<Context>().contentResolver) {
        val loaded = mutableListOf<ByteArray>()
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) =
            SelectedFile(uri, fallbackName, fallbackMime, 1)
        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(9).also { loaded += it })
        override fun loadBounded(file: SelectedFile, maximumBytes: Int) = loadDocument(file)
        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8, 8))
        override fun write(uri: Uri, bytes: ByteArray) = Unit
    }

    private class FakeCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "Disponible")
        override val signingFormats = SignatureFormats.ALL
        override val documentServices = DocumentServices.ALL.toSet()
        var lastRecords = 0
        var lastEni: EniRequest? = null
        var eniFailure: String? = null

        override fun selectCertificate() = certificate()
        override fun importCertificate(data: ByteArray, password: CharArray) = certificate()
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String) =
            SignedOutput(byteArrayOf(1), "firmado.p7s", "application/pkcs7-signature", "CAdES", "SHA256withRSA")
        override fun sealPreview(certificateId: String, options: Map<String, String>) = byteArrayOf()
        override fun verify(document: LoadedFile, original: LoadedFile?) = VerificationSummary(false, "", emptyList(),
            emptyList(), "", "", "unknown", "unknown", "unknown", "not_available", emptyList(), emptyList())
        override fun clearSession() = Unit

        override fun validateVeriFactu(records: List<LoadedFile>): VeriFactuReport {
            lastRecords = records.size
            return VeriFactuReport(false, 1, 0, listOf(VeriFactuRecord("registro.xml", "RegistroAlta", "AA", "BB", "",
                false, false, listOf(EngineIssue("Huella", "verifactu.hash", "error")))))
        }

        override fun createEniDocument(signature: LoadedFile, original: LoadedFile?, request: EniRequest): EniDocument {
            eniFailure?.let { throw CoreContractException(it) }
            lastEni = request
            return EniDocument("<enidoc/>".encodeToByteArray(), "TF04")
        }

        override fun validateEni(document: LoadedFile) =
            EniValidation(false, listOf(EngineIssue("documento", "eni.validacion.structure", "error")))

        override fun csvLegend(code: String, url: String, text: String) = CsvLegend("https://sede.example", "CSV")

        private fun certificate() = CertificateSummary("id", "Titular", "Emisor", "huella")
    }

    private companion object {
        val DOCUMENT_URI: Uri = Uri.parse("content://test/document")
        val CERTIFICATE_URI: Uri = Uri.parse("content://test/certificate")
        val RECORD_URI: Uri = Uri.parse("content://test/record")
        val OTHER_URI: Uri = Uri.parse("content://test/other")
        val ENI_URI: Uri = Uri.parse("content://test/eni")
    }
}
