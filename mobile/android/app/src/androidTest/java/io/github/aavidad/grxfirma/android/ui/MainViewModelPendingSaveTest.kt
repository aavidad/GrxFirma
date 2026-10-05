// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import android.net.Uri
import androidx.test.core.app.ApplicationProvider
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreBridge
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.files.ContentRepository
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
import io.github.aavidad.grxfirma.android.model.VerificationSummary
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(AndroidJUnit4::class)
class MainViewModelPendingSaveTest {
    @After
    fun resetDispatcher() {
        Dispatchers.resetMain()
    }

    @Test
    fun saveFailureKeepsSignedBytesAndExposesRecoveryActions() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val repository = FakeRepository(failWrites = 1)
        val core = FakeCore()
        val viewModel = preparedViewModel(repository, core, dispatcher)

        viewModel.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()

        assertTrue(viewModel.state.value.awaitingSave)
        assertTrue(viewModel.state.value.canRetryPendingOutput)
        assertTrue(viewModel.state.value.canDiscardPendingOutput)
        assertArrayEquals(SIGNED_BYTES, core.output.bytes)
        assertEquals(
            OperationResult.Error(UiText.Resource(R.string.error_save_failed_keep_output)),
            viewModel.state.value.result,
        )
    }

    @Test
    fun successfulSaveClearsPendingSignedBytes() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val repository = FakeRepository()
        val core = FakeCore()
        val viewModel = preparedViewModel(repository, core, dispatcher)

        viewModel.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()

        assertFalse(viewModel.state.value.awaitingSave)
        assertArrayEquals(SIGNED_BYTES, repository.writes.single())
        assertArrayEquals(ByteArray(SIGNED_BYTES.size), core.output.bytes)
        assertEquals(OperationResult.Success(UiText.Resource(R.string.result_saved)), viewModel.state.value.result)
    }

    @Test
    fun retrySavesRetainedOutputWithoutSigningAgain() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val repository = FakeRepository(failWrites = 1)
        val core = FakeCore()
        val viewModel = preparedViewModel(repository, core, dispatcher)

        viewModel.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()
        viewModel.retryPendingOutput()
        advanceUntilIdle()
        viewModel.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()

        assertEquals(1, core.signCalls)
        assertEquals(1, repository.writes.size)
        assertFalse(viewModel.state.value.awaitingSave)
        assertArrayEquals(ByteArray(SIGNED_BYTES.size), core.output.bytes)
    }

    @Test
    fun destinationPickerCancellationKeepsOutputActionable() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val core = FakeCore()
        val viewModel = preparedViewModel(FakeRepository(), core, dispatcher)

        viewModel.reportSavePickerCancelled()

        assertTrue(viewModel.state.value.awaitingSave)
        assertTrue(viewModel.state.value.canRetryPendingOutput)
        assertTrue(viewModel.state.value.canDiscardPendingOutput)
        assertArrayEquals(SIGNED_BYTES, core.output.bytes)
        assertEquals(
            OperationResult.Error(UiText.Resource(R.string.error_save_picker_cancelled)),
            viewModel.state.value.result,
        )
    }

    @Test
    fun discardClearsRetainedOutputAndSessionCertificate() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val repository = FakeRepository()
        val core = FakeCore()
        val viewModel = preparedViewModel(repository, core, dispatcher)

        viewModel.discardPendingOutput()

        assertFalse(viewModel.state.value.awaitingSave)
        assertEquals(null, viewModel.state.value.certificate)
        assertEquals(1, core.clearSessionCalls)
        assertArrayEquals(ByteArray(SIGNED_BYTES.size), core.output.bytes)
        assertEquals(
            OperationResult.Notice(UiText.Resource(R.string.result_save_cancelled)),
            viewModel.state.value.result,
        )
    }

    @Test
    fun automaticVerificationKeepsItsVerdictAfterSaving() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val core = FakeCore()
        val vm = preparedViewModel(FakeRepository(), core, dispatcher)
        assertEquals(1, core.signedVerificationCalls)
        assertFalse(core.usedOriginalForSignedOutput) // PAdES is embedded.
        assertFalse(vm.state.value.verification!!.toUiText().accredited())
        vm.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()
        assertEquals("valid", vm.state.value.verification!!.integrityStatus)
        assertTrue(vm.state.value.canExportReport)
    }

    @Test
    fun failedAutomaticVerificationStillAllowsSavingTheSignature() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val core = FakeCore().apply { automaticVerificationFails = true }
        val vm = preparedViewModel(FakeRepository(), core, dispatcher)
        assertTrue(vm.state.value.postSignVerificationFailed)
        assertTrue(vm.state.value.canRetryPendingOutput)
        assertEquals(null, vm.state.value.verification)
        vm.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()
        assertFalse(vm.state.value.awaitingSave)
        assertTrue(vm.state.value.postSignVerificationFailed)
    }

    @Test
    fun signedInputSuggestsCoSignAndForwardsProfileAndTSA() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val core = FakeCore()
        val vm = preparedViewModel(FakeRepository(), core, dispatcher) {
            assertTrue(it.state.value.coSignSuggested)
            it.acceptCoSignSuggestion()
            it.updateSigningSettings("cosign", "t", true, "http://tsa.example/rfc3161")
        }
        assertEquals("cosign", core.lastAction)
        assertEquals("t", core.lastOptions["profile"])
        assertEquals("http://tsa.example/rfc3161", core.lastOptions["tsaURL"])
        assertTrue(vm.state.value.awaitingSave)
    }

    @Test
    fun reportExportNeverWritesSignatureBytesAndSurvivesCancellation() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(dispatcher)
        val repository = FakeRepository()
        val vm = preparedViewModel(repository, FakeCore(), dispatcher)
        vm.savePendingOutput(DESTINATION_URI)
        advanceUntilIdle()
        vm.exportVerificationReport()
        assertTrue(vm.state.value.awaitingReportSave)
        assertFalse(vm.state.value.canReplaceSelection)
        vm.cancelReportExport()
        assertTrue(vm.state.value.canExportReport)
        vm.exportVerificationReport()
        vm.saveVerificationReport(DESTINATION_URI)
        advanceUntilIdle()
        assertEquals(vm.state.value.verification!!.reportJson, repository.writes.last().decodeToString())
        assertFalse(vm.state.value.awaitingReportSave)
        assertEquals(OperationResult.Success(UiText.Resource(R.string.result_report_saved)), vm.state.value.result)
    }

    private fun TestScope.preparedViewModel(
        repository: FakeRepository,
        core: FakeCore,
        dispatcher: TestDispatcher,
        beforeSign: (MainViewModel) -> Unit = {},
    ): MainViewModel {
        val viewModel = MainViewModel(repository, core, dispatcher)
        viewModel.selectDocument(DOCUMENT_URI)
        advanceUntilIdle()
        viewModel.selectCertificateFile(CERTIFICATE_URI)
        viewModel.importCertificate(charArrayOf('p'))
        advanceUntilIdle()
        beforeSign(viewModel)
        viewModel.sign("pades")
        advanceUntilIdle()
        return viewModel
    }

    private class FakeRepository(failWrites: Int = 0) : ContentRepository(ApplicationProvider.getApplicationContext<Context>().contentResolver) {
        var remainingWriteFailures = failWrites
        val writes = mutableListOf<ByteArray>()

        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) = SelectedFile(
            uri = uri,
            displayName = fallbackName,
            mimeType = fallbackMime,
            sizeBytes = 1,
        )

        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(9))

        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8))

        override fun write(uri: Uri, bytes: ByteArray) {
            if (remainingWriteFailures > 0) {
                remainingWriteFailures -= 1
                throw SecurityException("Proveedor rechazó el destino")
            }
            writes += bytes.copyOf()
        }
    }

    private class FakeCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "Disponible")
        var automaticVerificationFails = false
        var signedVerificationCalls = 0
        var lastAction = ""
        var lastOptions = emptyMap<String, String>()
        var usedOriginalForSignedOutput = false
        var signCalls = 0
        var clearSessionCalls = 0
        val output = SignedOutput(SIGNED_BYTES.copyOf(), "firmado.pdf", "application/pdf", "PAdES", "SHA-256")

        override fun selectCertificate(): CertificateSummary = certificate()

        override fun importCertificate(data: ByteArray, password: CharArray): CertificateSummary = certificate()

        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput {
            signCalls += 1
            lastAction = action
            lastOptions = options
            return output
        }

        override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray = byteArrayOf()

        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary {
            if (document.bytes.contentEquals(SIGNED_BYTES)) {
                signedVerificationCalls++
                usedOriginalForSignedOutput = original != null
                if (automaticVerificationFails) throw IllegalStateException("Verification failed")
            }
            return VerificationSummary(false, "", listOf("evidence"), listOf("id"), "PAdES", "full",
                "valid", "unknown", "unknown", "embedded_evidence_only", listOf("warning"), emptyList(),
                reportJson = "{\"valid\":false,\"signers\":[\"id\"]}")
        }

        override fun clearSession() {
            clearSessionCalls += 1
        }

        private fun certificate() = CertificateSummary("id", "Titular", "Emisor", "huella")
    }

    private companion object {
        val DOCUMENT_URI: Uri = Uri.parse("content://test/document")
        val CERTIFICATE_URI: Uri = Uri.parse("content://test/certificate")
        val DESTINATION_URI: Uri = Uri.parse("content://test/destination")
        val SIGNED_BYTES = byteArrayOf(1, 2, 3)
    }
}
