// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.net.FakeContentUri
import android.net.Uri
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreBridge
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.files.DocumentFingerprint
import io.github.aavidad.grxfirma.android.files.DocumentRepository
import io.github.aavidad.grxfirma.android.model.BatchItemResult
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
import io.github.aavidad.grxfirma.android.model.VerificationSummary
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * H-06: Android firma lo mismo que se eligió. Al elegir el documento se lee
 * una vez y se guarda su SHA-256; al firmar se vuelve a leer y, si el
 * proveedor devuelve otro contenido, no se llama al núcleo.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModelTest {
    private val dispatcher = UnconfinedTestDispatcher()

    /** Proveedor que entrega una lista de contenidos, uno por lectura. */
    private class ChangingRepository(vararg contents: ByteArray) : DocumentRepository {
        private val queue = ArrayDeque(contents.toList())
        val handedOut = mutableListOf<ByteArray>()
        var reads = 0
        var failFirstRead = false

        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) =
            SelectedFile(uri, "contrato.txt", "text/plain", 8)

        override fun loadDocument(file: SelectedFile): LoadedFile {
            reads++
            if (failFirstRead && reads == 1) throw java.io.IOException("proveedor no disponible")
            val bytes = (if (queue.size > 1) queue.removeFirst() else queue.first()).copyOf()
            handedOut += bytes
            return LoadedFile(file.displayName, file.mimeType, bytes)
        }

        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8))
        override fun write(uri: Uri, bytes: ByteArray) = Unit
    }

    private class CountingCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "")
        var signCalls = 0
        val signed = mutableListOf<ByteArray>()
        override fun selectCertificate(): CertificateSummary = error("no")
        override fun importCertificate(data: ByteArray, password: CharArray) = CertificateSummary("id", "Titular", "Emisor", "huella")
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput {
            signCalls++
            signed += document.bytes.copyOf()
            return SignedOutput(byteArrayOf(1, 2, 3), "contrato.csig", "application/pkcs7-signature", "CAdES", "SHA-256")
        }
        override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray = byteArrayOf()
        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary = error("sin verificación")
        override fun clearSession() = Unit
    }

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    private fun ready(repository: ChangingRepository, core: CountingCore): MainViewModel {
        val vm = MainViewModel(repository, core, dispatcher)
        vm.selectDocument(FakeContentUri("content://test/document"))
        vm.selectCertificateFile(FakeContentUri("content://test/certificate"))
        vm.importCertificate(charArrayOf('p'))
        assertNotNull(vm.state.value.certificate)
        return vm
    }

    private val chosen = "contrato".toByteArray()
    private val swapped = "otra cosa".toByteArray()

    @Test fun `a provider that changes the bytes after the choice aborts without calling the core`() {
        val repository = ChangingRepository(chosen, swapped)
        val core = CountingCore()
        val vm = ready(repository, core)
        assertEquals(DocumentFingerprint.sha256Hex(chosen), vm.state.value.document?.sha256)

        vm.sign("cades")

        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_document_changed)), vm.state.value.result)
        assertEquals(0, core.signCalls)
        assertEquals(2, repository.reads)
        repository.handedOut.forEach { assertArrayEquals(ByteArray(it.size), it) }
    }

    @Test fun `the chosen document is read once at selection and signed when it has not changed`() {
        val repository = ChangingRepository(chosen)
        val core = CountingCore()
        val vm = ready(repository, core)
        assertEquals(1, repository.reads)

        vm.sign("cades")

        assertEquals(1, core.signCalls)
        assertArrayEquals(chosen, core.signed.single())
        assertEquals(2, repository.reads)
    }

    @Test fun `a document that could not be read when chosen is not signed`() {
        val repository = ChangingRepository(chosen).apply { failFirstRead = true }
        val core = CountingCore()
        val vm = ready(repository, core)
        assertEquals(null, vm.state.value.document?.sha256)

        vm.sign("cades")

        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_document_not_read)), vm.state.value.result)
        assertEquals(0, core.signCalls)
    }

    /** Lote: cada fichero (por el último segmento del Uri) entrega su lista de contenidos. */
    private class BatchRepository(private val contents: Map<String, List<ByteArray>>) : DocumentRepository {
        private val reads = HashMap<String, Int>()
        val handedOut = mutableListOf<ByteArray>()
        val failAtSelection = mutableSetOf<String>()

        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) =
            SelectedFile(uri, uri.lastPathSegment!!, "text/plain", 8)

        override fun loadDocument(file: SelectedFile): LoadedFile {
            val name = file.displayName
            val read = reads.getOrDefault(name, 0)
            reads[name] = read + 1
            if (read == 0 && name in failAtSelection) throw java.io.IOException("proveedor no disponible")
            val versions = contents.getValue(name)
            val bytes = versions[minOf(read, versions.size - 1)].copyOf()
            handedOut += bytes
            return LoadedFile(name, file.mimeType, bytes)
        }

        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8))
        override fun write(uri: Uri, bytes: ByteArray) = Unit
    }

    private class BatchCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "")
        override val toolsAvailable = true
        val batches = mutableListOf<List<Pair<String, ByteArray>>>()
        override fun selectCertificate(): CertificateSummary = error("no")
        override fun importCertificate(data: ByteArray, password: CharArray) = CertificateSummary("id", "Titular", "Emisor", "huella")
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput =
            error("el lote no firma de uno en uno")
        override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray = byteArrayOf()
        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary = error("sin verificación")
        override fun clearSession() = Unit
        override fun signBatch(documents: List<LoadedFile>, format: String, certificateId: String, options: Map<String, String>): List<BatchItemResult> {
            batches += documents.map { it.displayName to it.bytes.copyOf() }
            return documents.map {
                BatchItemResult(it.displayName, SignedOutput(byteArrayOf(1), it.displayName + ".csig", "application/pkcs7-signature", "CAdES", "SHA-256"))
            }
        }
    }

    private fun readyBatch(repository: BatchRepository, core: BatchCore, vararg names: String): MainViewModel {
        val vm = MainViewModel(repository, core, dispatcher)
        vm.selectCertificateFile(FakeContentUri("content://test/certificate"))
        vm.importCertificate(charArrayOf('p'))
        vm.selectBatchDocuments(names.map { FakeContentUri("content://test/$it") })
        assertTrue(vm.state.value.canSignBatch)
        return vm
    }

    @Test fun `a batch file that changes after the choice is not signed and the rest is`() {
        val other = "acta".toByteArray()
        val repository = BatchRepository(mapOf(
            "contrato.txt" to listOf(chosen, swapped),
            "acta.txt" to listOf(other),
        ))
        val core = BatchCore()
        val vm = readyBatch(repository, core, "contrato.txt", "acta.txt")
        assertEquals(listOf(DocumentFingerprint.sha256Hex(chosen), DocumentFingerprint.sha256Hex(other)),
            vm.state.value.batchDocuments.map { it.sha256 })

        vm.signBatch("cades")

        // Al núcleo solo llega el fichero que no cambió, con los bytes comprobados.
        assertEquals(listOf("acta.txt"), core.batches.single().map { it.first })
        assertArrayEquals(other, core.batches.single().single().second)
        val result = vm.state.value.result as OperationResult.Success
        val lines = (result.detail as UiText.Lines).lines
        assertTrue(UiText.Plural(R.plurals.batch_ok_count, 1) in lines)
        assertTrue(UiText.Plural(R.plurals.batch_error_count, 1) in lines)
        assertTrue(UiText.Resource(R.string.batch_item_changed, listOf("contrato.txt")) in lines)
        assertTrue(UiText.Resource(R.string.batch_item_ok, listOf("acta.txt")) in lines)
        // El orden del lote se respeta: primero el contrato, después el acta.
        val items = lines.filterIsInstance<UiText.Resource>().map { it.arguments.single() }
        assertEquals(listOf("contrato.txt", "acta.txt"), items)
        assertTrue(vm.state.value.awaitingSave)
        repository.handedOut.forEach { assertArrayEquals(ByteArray(it.size), it) }
    }

    @Test fun `a batch where every file changed does not call the core`() {
        val repository = BatchRepository(mapOf("contrato.txt" to listOf(chosen, swapped)))
        val core = BatchCore()
        val vm = readyBatch(repository, core, "contrato.txt")

        vm.signBatch("cades")

        assertTrue(core.batches.isEmpty())
        val lines = ((vm.state.value.result as OperationResult.Error).detail as UiText.Lines).lines
        assertTrue(UiText.Resource(R.string.batch_item_changed, listOf("contrato.txt")) in lines)
        assertEquals(false, vm.state.value.awaitingSave)
    }

    @Test fun `a batch file that could not be read when chosen is not signed`() {
        val other = "acta".toByteArray()
        val repository = BatchRepository(mapOf(
            "contrato.txt" to listOf(chosen),
            "acta.txt" to listOf(other),
        )).apply { failAtSelection += "contrato.txt" }
        val core = BatchCore()
        val vm = readyBatch(repository, core, "contrato.txt", "acta.txt")
        assertEquals(null, vm.state.value.batchDocuments.first().sha256)

        vm.signBatch("cades")

        assertEquals(listOf("acta.txt"), core.batches.single().map { it.first })
        val lines = ((vm.state.value.result as OperationResult.Success).detail as UiText.Lines).lines
        assertTrue(UiText.Resource(R.string.batch_item_not_read, listOf("contrato.txt")) in lines)
    }

    @Test fun `the summary shows the first sixteen hex characters of the fingerprint`() {
        val digest = DocumentFingerprint.sha256Hex(chosen)
        assertEquals(64, digest.length)
        assertEquals(digest.take(16), DocumentFingerprint.visiblePrefix(digest))
        assertEquals(true, DocumentFingerprint.matches(digest.uppercase(), chosen))
        assertEquals(false, DocumentFingerprint.matches(digest, swapped))
    }
}
