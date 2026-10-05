// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.ui

import android.content.Context
import android.net.Uri
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreBridge
import es.dipgra.grxfirma.android.core.CoreReadiness
import es.dipgra.grxfirma.android.files.ContentRepository
import es.dipgra.grxfirma.android.model.BatchItemResult
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.HashCheck
import es.dipgra.grxfirma.android.model.HashOutput
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.ProtectionRequest
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.model.SignedOutput
import es.dipgra.grxfirma.android.model.VerificationSummary
import java.util.Base64
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
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(AndroidJUnit4::class)
class MainViewModelToolsTest {
    @After fun resetDispatcher() = Dispatchers.resetMain()

    @Test fun hashIsSavedWithDesktopNameAndKeepsTheSessionOnDiscard() = runTest {
        val (vm, repository, core) = ready()
        vm.updateToolSettings("SHA-512", "base64", "cms", true)
        vm.createHash()
        advanceUntilIdle()
        assertEquals("SHA-512" to "base64", core.lastHash)
        assertTrue(vm.state.value.awaitingSave)
        assertEquals(PendingKind.TOOL, vm.state.value.pendingKind)
        vm.discardPendingOutput()
        assertEquals(0, core.clearSessionCalls)
        assertEquals("id", vm.state.value.certificate?.id)
        assertEquals(OperationResult.Error(UiText.Resource(R.string.result_tool_discarded)), vm.state.value.result)
        vm.createHash()
        advanceUntilIdle()
        vm.savePendingOutput(DESTINATION)
        advanceUntilIdle()
        assertArrayEquals(HASH_FILE, repository.writes.single())
        assertEquals(R.string.result_file_saved, ((vm.state.value.result as OperationResult.Success).title as UiText.Resource).id)
    }

    @Test fun hashCheckReportsMismatchAsError() = runTest {
        val (vm, _, core) = ready()
        core.hashMatches = false
        vm.checkHash(HASH_URI)
        advanceUntilIdle()
        assertTrue(vm.state.value.result is OperationResult.Error)
        core.hashMatches = true
        vm.checkHash(HASH_URI)
        advanceUntilIdle()
        assertEquals(R.string.result_hash_match,
            ((vm.state.value.result as OperationResult.Success).title as UiText.Resource).id)
    }

    @Test fun protectForRecipientsSendsCertificatesAndEncryptedDataNeedsMatchingKey() = runTest {
        val (vm, _, core) = ready()
        vm.addRecipient(RECIPIENT_URI)
        vm.protect(CharArray(0), CharArray(0), sign = false)
        advanceUntilIdle()
        val request = core.lastProtection!!
        assertEquals("cms", request.container)
        assertTrue(request.includeSessionCertificate)
        assertEquals(1, request.recipients.size)
        assertArrayEquals(ByteArray(request.recipients[0].size), request.recipients[0]) // borrado tras usarse
        vm.discardPendingOutput()

        vm.updateToolSettings("SHA-256", "hex", "cms-encrypted", true)
        val key = Base64.getEncoder().encodeToString(ByteArray(32) { 3 }).toCharArray()
        val wrong = Base64.getEncoder().encodeToString(ByteArray(32) { 4 }).toCharArray()
        vm.protect(key.copyOf(), wrong, sign = false)
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_protect_key)), vm.state.value.result)
        assertArrayEquals(CharArray(44), wrong)
        val secret = key.copyOf()
        vm.protect(secret, key.copyOf(), sign = false)
        advanceUntilIdle()
        assertEquals(String(key), core.lastSecret)
        assertArrayEquals(CharArray(44), secret)
        assertFalse(core.lastProtection!!.includeSessionCertificate)
    }

    @Test fun protectAndSignAndBatchAreRefusedForTheDnie() = runTest {
        val (vm, _, _) = ready()
        vm.selectBatchDocuments(listOf(DOCUMENT_URI, OTHER_URI))
        assertTrue(vm.state.value.canSignBatch)
        // Simula el estado tras instalar un DNIe sin abrir NFC.
        val field = MainViewModel::class.java.getDeclaredField("mutableState").apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val flow = field.get(vm) as kotlinx.coroutines.flow.MutableStateFlow<MainUiState>
        flow.value = flow.value.copy(certificateExternal = true)
        vm.signBatch("auto")
        assertEquals(OperationResult.Error(UiText.Resource(R.string.batch_dnie_unavailable)), vm.state.value.result)
        vm.protect(CharArray(0), CharArray(0), sign = true)
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_protect_sign_identity)), vm.state.value.result)
    }

    @Test fun batchReportsEachFileAndSavesIntoTheChosenFolder() = runTest {
        val (vm, repository, core) = ready()
        vm.selectBatchDocuments(listOf(DOCUMENT_URI, OTHER_URI))
        vm.signBatch("cades")
        advanceUntilIdle()
        assertEquals(2, core.lastBatchSize)
        assertEquals(PendingKind.BATCH, vm.state.value.pendingKind)
        assertTrue(vm.state.value.awaitingSave)
        repository.failTreeWrites = 1
        vm.saveBatchOutputs(FOLDER)
        advanceUntilIdle()
        assertTrue(vm.state.value.awaitingSave) // la firma no guardada sigue en memoria
        vm.saveBatchOutputs(FOLDER)
        advanceUntilIdle()
        assertFalse(vm.state.value.awaitingSave)
        assertEquals(listOf("documento-firmado.p7s"), repository.treeWrites)
        val result = vm.state.value.result as OperationResult.Success
        assertEquals(UiText.Plural(R.plurals.batch_saved_count, 1), result.title)
    }

    @Test fun batchRejectsMoreThanSixteenFiles() = runTest {
        val (vm, _, _) = ready()
        vm.selectBatchDocuments((0..16).map { Uri.parse("content://test/doc$it") })
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_batch_too_many)), vm.state.value.result)
        assertTrue(vm.state.value.batchDocuments.isEmpty())
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
        assertTrue(vm.state.value.canUseTools)
        return Triple(vm, repository, core)
    }

    private class FakeRepository : ContentRepository(ApplicationProvider.getApplicationContext<Context>().contentResolver) {
        val writes = mutableListOf<ByteArray>()
        val treeWrites = mutableListOf<String>()
        var failTreeWrites = 0

        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) =
            SelectedFile(uri, fallbackName, fallbackMime, 1)

        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(9))

        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8, 8))

        override fun write(uri: Uri, bytes: ByteArray) {
            writes += bytes.copyOf()
        }

        override fun writeToTree(folder: Uri, displayName: String, mimeType: String, bytes: ByteArray) {
            if (failTreeWrites > 0) {
                failTreeWrites--
                throw SecurityException("carpeta no disponible")
            }
            treeWrites += displayName
        }
    }

    private class FakeCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "Disponible")
        override val toolsAvailable = true
        var clearSessionCalls = 0
        var lastHash: Pair<String, String>? = null
        var hashMatches = true
        var lastProtection: ProtectionRequest? = null
        var lastSecret: String? = null
        var lastBatchSize = 0

        override fun selectCertificate() = certificate()
        override fun importCertificate(data: ByteArray, password: CharArray) = certificate()
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String) =
            SignedOutput(byteArrayOf(1), "firmado.p7s", "application/pkcs7-signature", "CAdES", "SHA256withRSA")
        override fun sealPreview(certificateId: String, options: Map<String, String>) = byteArrayOf()
        override fun verify(document: LoadedFile, original: LoadedFile?) = VerificationSummary(false, "", emptyList(),
            emptyList(), "", "", "unknown", "unknown", "unknown", "not_available", emptyList(), emptyList())
        override fun clearSession() { clearSessionCalls++ }

        override fun createHash(document: LoadedFile, algorithm: String, format: String): HashOutput {
            lastHash = algorithm to format
            return HashOutput(algorithm, format, "aGFzaA==", HASH_FILE.copyOf(), "hashb64")
        }

        override fun checkHash(document: LoadedFile, hashFile: LoadedFile) =
            HashCheck(hashMatches, "SHA-256", "aa", if (hashMatches) "aa" else "bb")

        override fun protect(document: LoadedFile, request: ProtectionRequest, secret: CharArray?): SignedOutput {
            lastProtection = request
            lastSecret = secret?.let { String(it) }
            secret?.fill('\u0000')
            return SignedOutput(byteArrayOf(5), "documento.enveloped", "application/pkcs7-mime", request.container, "")
        }

        override fun unprotect(document: LoadedFile, secret: CharArray?): SignedOutput {
            secret?.fill('\u0000')
            return SignedOutput(byteArrayOf(6), "documento", "application/octet-stream", "", "")
        }

        override fun signBatch(documents: List<LoadedFile>, format: String, certificateId: String, options: Map<String, String>): List<BatchItemResult> {
            lastBatchSize = documents.size
            return listOf(
                BatchItemResult(documents[0].displayName,
                    SignedOutput(byteArrayOf(7), "documento-firmado.p7s", "application/pkcs7-signature", "CAdES", "SHA256withRSA")),
                BatchItemResult(documents[1].displayName, null),
            )
        }

        private fun certificate() = CertificateSummary("id", "Titular", "Emisor", "huella")
    }

    private companion object {
        val DOCUMENT_URI: Uri = Uri.parse("content://test/document")
        val OTHER_URI: Uri = Uri.parse("content://test/other")
        val CERTIFICATE_URI: Uri = Uri.parse("content://test/certificate")
        val HASH_URI: Uri = Uri.parse("content://test/hash")
        val RECIPIENT_URI: Uri = Uri.parse("content://test/recipient")
        val DESTINATION: Uri = Uri.parse("content://test/destination")
        val FOLDER: Uri = Uri.parse("content://test/tree/folder")
        val HASH_FILE = "aGFzaA==".encodeToByteArray()
    }
}
