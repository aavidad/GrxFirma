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
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.core.DocumentServices
import io.github.aavidad.grxfirma.android.core.Wave4Capabilities
import io.github.aavidad.grxfirma.android.files.ContentRepository
import io.github.aavidad.grxfirma.android.model.BatchItemInput
import io.github.aavidad.grxfirma.android.model.BatchItemResult
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.EngineIssue
import io.github.aavidad.grxfirma.android.model.EniFileRequest
import io.github.aavidad.grxfirma.android.model.EniFileResult
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
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
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Expediente ENI y lote de la cuarta oleada con núcleo y almacenamiento simulados. */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(AndroidJUnit4::class)
class MainViewModelWave4Test {
    @After fun resetDispatcher() = Dispatchers.resetMain()

    @Test fun folderKeepsOnlyXmlSortedAndExpedienteIsSavedAsTool() = runTest {
        val (vm, repository, core) = ready()
        vm.selectEniFileFolder(FOLDER)
        advanceUntilIdle()
        assertEquals(listOf("a.xml", "b.xml"), vm.state.value.wave4.eniFileDocuments.map { it.displayName })
        assertEquals(1, vm.state.value.wave4.eniFileSkipped)
        vm.createEniFile(EniFileRequest(listOf("L01180877"), "L01180877_PRO_1", "E02"))
        advanceUntilIdle()
        assertEquals(listOf("a.xml", "b.xml"), core.lastEniFileNames)
        assertTrue(core.lastEniFileBytes.all { bytes -> bytes.all { it == 0.toByte() } }) // borrados tras usarse
        assertTrue(vm.state.value.awaitingSave)
        assertEquals(PendingKind.TOOL, vm.state.value.pendingKind)
        vm.savePendingOutput(DESTINATION)
        advanceUntilIdle()
        assertArrayEquals("<exp/>".toByteArray(), repository.writes.single())
    }

    @Test fun invalidDocumentsAreListedWithoutSigning() = runTest {
        val (vm, _, core) = ready()
        core.eniFileIssues = listOf(EngineIssue("b.xml", "eni.validacion.structure", "error"))
        vm.selectEniFileFolder(FOLDER)
        advanceUntilIdle()
        vm.createEniFile(EniFileRequest(listOf("L01180877"), "123", "E01"))
        advanceUntilIdle()
        val error = vm.state.value.result as OperationResult.Error
        val lines = (error.detail as UiText.Lines).lines
        assertEquals(UiText.Resource(R.string.expediente_invalid_documents), lines.first())
        assertEquals(1, lines.size - 1)
        assertNull(vm.state.value.takeIf { it.awaitingSave })
    }

    @Test fun metadataIsCheckedBeforeCallingTheCore() = runTest {
        val (vm, _, core) = ready()
        vm.selectEniFileFolder(FOLDER)
        advanceUntilIdle()
        vm.createEniFile(EniFileRequest(listOf("granada"), "123", "E01"))
        assertEquals(OperationResult.Error(UiText.Engine("eni.validacion.dir3")), vm.state.value.result)
        vm.createEniFile(EniFileRequest(listOf("L01180877"), "procedimiento", "E01"))
        assertEquals(OperationResult.Error(UiText.Engine("eni.validacion.classification")), vm.state.value.result)
        assertEquals(0, core.eniFileCalls)
    }

    @Test fun cosignBatchSendsEachItemWithItsActionAndDnieIsAccepted() = runTest {
        val (vm, _, core) = ready()
        vm.selectBatchDocuments(listOf(DOCUMENT_URI, OTHER_URI))
        val field = MainViewModel::class.java.getDeclaredField("mutableState").apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val flow = field.get(vm) as kotlinx.coroutines.flow.MutableStateFlow<MainUiState>
        flow.value = flow.value.copy(certificateExternal = true)
        vm.updateBatchOptions("cosign", seal = false)
        var finished = false
        vm.signBatch("cades", onFinished = { finished = true })
        advanceUntilIdle()
        assertEquals(listOf("cosign", "cosign"), core.lastItems.map { it.action })
        assertTrue(finished)
        assertEquals(R.string.result_batch_cosigned,
            ((vm.state.value.result as OperationResult.Success).title as UiText.Resource).id)
    }

    @Test fun sealedBatchAsksThePlannerOnlyForPdfs() = runTest {
        val (vm, _, core) = ready()
        vm.selectBatchDocuments(listOf(PDF_URI, DOCUMENT_URI))
        vm.updateBatchOptions("sign", seal = true)
        val planned = mutableListOf<String>()
        vm.signBatch("auto", BatchSealPlanner { file -> planned += file.displayName; mapOf("visibleSeal" to "true") })
        advanceUntilIdle()
        assertEquals(listOf("documento.pdf"), planned)
        assertEquals(mapOf("visibleSeal" to "true"), core.lastItems.first().options)
        assertTrue(core.lastItems.last().options.isEmpty())
    }

    private fun TestScope.ready(): Triple<MainViewModel, FakeRepository, FakeCore> {
        Dispatchers.setMain(StandardTestDispatcher(testScheduler))
        val repository = FakeRepository()
        val core = FakeCore()
        val vm = MainViewModel(repository, core, StandardTestDispatcher(testScheduler))
        vm.selectCertificateFile(CERTIFICATE_URI)
        vm.importCertificate(charArrayOf('p'))
        advanceUntilIdle()
        return Triple(vm, repository, core)
    }

    private class FakeRepository : ContentRepository(ApplicationProvider.getApplicationContext<Context>().contentResolver) {
        val writes = mutableListOf<ByteArray>()

        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) = when (uri) {
            PDF_URI -> SelectedFile(uri, "documento.pdf", "application/pdf", 1)
            else -> SelectedFile(uri, "documento.txt", "text/plain", 1)
        }

        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(9))

        override fun loadBounded(file: SelectedFile, maximumBytes: Int) =
            LoadedFile(file.displayName, file.mimeType, "<documento/>".toByteArray())

        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8))

        override fun write(uri: Uri, bytes: ByteArray) {
            writes += bytes.copyOf()
        }

        override fun listTree(folder: Uri, maximumEntries: Int) = listOf(
            SelectedFile(Uri.parse("content://test/tree/b"), "b.xml", "text/xml", 10),
            SelectedFile(Uri.parse("content://test/tree/n"), "notas.txt", "text/plain", 10),
            SelectedFile(Uri.parse("content://test/tree/a"), "a.xml", "application/octet-stream", 10),
        )
    }

    private class FakeCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "Disponible")
        override val toolsAvailable = true
        override val documentServices = setOf(DocumentServices.ENI_FILE)
        override val capabilities = Wave4Capabilities.ALL.toSet()
        var eniFileCalls = 0
        var eniFileIssues: List<EngineIssue> = emptyList()
        var lastEniFileNames: List<String> = emptyList()
        var lastEniFileBytes: List<ByteArray> = emptyList()
        var lastItems: List<BatchItemInput> = emptyList()

        override fun selectCertificate() = certificate()
        override fun importCertificate(data: ByteArray, password: CharArray) = certificate()
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String) =
            SignedOutput(byteArrayOf(1), "firmado.p7s", "application/pkcs7-signature", "CAdES", "SHA256withRSA")
        override fun sealPreview(certificateId: String, options: Map<String, String>) = byteArrayOf()
        override fun verify(document: LoadedFile, original: LoadedFile?) = VerificationSummary(false, "", emptyList(),
            emptyList(), "", "", "unknown", "unknown", "unknown", "not_available", emptyList(), emptyList())
        override fun clearSession() = Unit

        override fun createEniFile(documents: List<LoadedFile>, certificateId: String, request: EniFileRequest): EniFileResult {
            eniFileCalls++
            lastEniFileNames = documents.map { it.displayName }
            lastEniFileBytes = documents.map { it.bytes }
            return if (eniFileIssues.isEmpty()) EniFileResult("<exp/>".toByteArray(), documents.size, emptyList())
            else EniFileResult(null, documents.size, eniFileIssues)
        }

        override fun signBatchItems(items: List<BatchItemInput>, certificateId: String, options: Map<String, String>): List<BatchItemResult> {
            lastItems = items.map { BatchItemInput(LoadedFile(it.file.displayName, it.file.mimeType, ByteArray(0)), it.format, it.action, it.options) }
            return items.map {
                BatchItemResult(it.file.displayName,
                    SignedOutput(byteArrayOf(7), it.file.displayName + ".p7s", "application/pkcs7-signature", "CAdES", "SHA256withRSA"))
            }
        }

        private fun certificate() = CertificateSummary("id", "Titular", "Emisor", "huella")
    }

    private companion object {
        val DOCUMENT_URI: Uri = Uri.parse("content://test/document")
        val OTHER_URI: Uri = Uri.parse("content://test/other")
        val PDF_URI: Uri = Uri.parse("content://test/pdf")
        val CERTIFICATE_URI: Uri = Uri.parse("content://test/certificate")
        val DESTINATION: Uri = Uri.parse("content://test/destination")
        val FOLDER: Uri = Uri.parse("content://test/tree/folder")
    }
}
