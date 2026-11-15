// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.net.Uri
import io.github.aavidad.grxfirma.android.core.*
import io.github.aavidad.grxfirma.android.files.DocumentRepository
import io.github.aavidad.grxfirma.android.model.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.After
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModelUnsafeExceptionsTest {
    @After fun resetDispatcher() = Dispatchers.resetMain()

    @Test fun typedBlockRequiresConsentAndRetriesVerificationWithTheSameDocument() = runTest {
        val (vm, core) = ready()
        vm.verify()
        advanceUntilIdle()
        val blocked = vm.state.value.unsafePrompt!!
        assertTrue(blocked.canActivate)
        assertEquals(0, core.activations)
        assertEquals(1, core.verifications)
        vm.activateUnsafeException(false)
        advanceUntilIdle()
        assertEquals(0, core.activations)
        vm.activateUnsafeException(true)
        advanceUntilIdle()
        assertEquals(1, core.activations)
        assertEquals("core.version.2", core.versionAccepted)
        assertEquals(2, core.verifications)
        assertNull(vm.state.value.unsafePrompt)
        assertNotNull(vm.state.value.verification)
        assertTrue(vm.state.value.result is OperationResult.Success)
    }

    @Test fun cancellationDoesNotEnableAnythingAndAnOrganizationBlockCannotBeAccepted() = runTest {
        val (vm, core) = ready()
        vm.verify()
        advanceUntilIdle()
        vm.dismissUnsafePrompt()
        assertNull(vm.state.value.unsafePrompt)
        assertEquals(0, core.activations)
        core.activable = false
        vm.verify()
        advanceUntilIdle()
        assertFalse(vm.state.value.unsafePrompt!!.canActivate)
        vm.activateUnsafeException(true)
        advanceUntilIdle()
        assertEquals(0, core.activations)
    }

    @Test fun policyChangeWhileAcceptingIsShownAndDoesNotRetry() = runTest {
        val (vm, core) = ready()
        vm.verify()
        advanceUntilIdle()
        core.activable = false
        vm.activateUnsafeException(true)
        advanceUntilIdle()
        assertEquals(1, core.verifications)
        assertNotNull(vm.state.value.unsafeError)
        assertFalse(vm.state.value.unsafePrompt!!.canActivate)
    }

    @Test fun newTextVersionRequiresReadingAndAcceptingAgain() = runTest {
        val (vm, core) = ready()
        vm.verify()
        advanceUntilIdle()
        core.version = "core.version.3"
        vm.activateUnsafeException(true)
        advanceUntilIdle()
        assertEquals(0, core.activations)
        assertEquals("core.version.3", vm.state.value.unsafePrompt!!.acceptanceVersion)
        assertNotNull(vm.state.value.unsafeError)
        vm.activateUnsafeException(true)
        advanceUntilIdle()
        assertEquals("core.version.3", core.versionAccepted)
        assertEquals(2, core.verifications)
    }

    @Test fun importingWithAPasswordDoesNotRetainOrReplayThatPassword() = runTest {
        val (vm, core) = ready()
        vm.selectCertificateFile(Uri.parse("content://test/certificate"))
        val password = charArrayOf('p')
        core.blockImport = true
        vm.importCertificate(password)
        advanceUntilIdle()
        assertArrayEquals(CharArray(1), password)
        assertNotNull(vm.state.value.unsafePrompt)
        vm.activateUnsafeException(true)
        advanceUntilIdle()
        assertEquals(1, core.imports)
        assertEquals(OperationResult.Notice(UiText.Engine("android.inseguro.reenviar")), vm.state.value.result)
    }

    private fun TestScope.ready(): Pair<MainViewModel, FakeCore> {
        Dispatchers.setMain(StandardTestDispatcher(testScheduler))
        val core = FakeCore()
        val vm = MainViewModel(FakeRepository(), core, StandardTestDispatcher(testScheduler))
        vm.selectDocument(Uri.parse("content://test/document"))
        advanceUntilIdle()
        return vm to core
    }

    private class FakeRepository : DocumentRepository {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) = SelectedFile(uri, fallbackName, fallbackMime, 1)
        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(1))
        override fun loadCertificate(file: SelectedFile) = loadDocument(file)
        override fun write(uri: Uri, bytes: ByteArray) = Unit
    }

    private class FakeCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "")
        override val unsafeExceptionsAvailable = true
        var activable = true
        var enabled = false
        var activations = 0
        var verifications = 0
        var versionAccepted = ""
        var version = "core.version.2"
        var blockImport = false
        var imports = 0
        private fun item(id: String = "sha1_leer") = UnsafeExceptionState(id,
            enabled = enabled && id == "sha1_leer", fixed = !activable, activable = activable,
            acceptanceVersion = version)
        override fun unsafeExceptions() = UnsafeExceptions.ids.map { item(it) }
        override fun activateUnsafeException(item: UnsafeExceptionState, accepted: Boolean) {
            if (!activable || !accepted) throw CoreContractException("excepciones.sha1_leer.organizacion")
            if (item.acceptanceVersion != version) throw CoreContractException("excepciones.version_aceptacion_invalida")
            activations++
            versionAccepted = item.acceptanceVersion
            enabled = true
        }
        override fun inspectSignature(document: LoadedFile) = SignatureInspection(false)
        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary {
            verifications++
            if (!enabled) throw UnsafeExceptionBlockedException(item().copy(acceptanceVersion = ""))
            return VerificationSummary(false, "", emptyList(), emptyList(), "CAdES", "", "unknown", "unknown", "unknown",
                "not_available", listOf("excepciones.sha1_leer.aviso"), emptyList())
        }
        override fun selectCertificate() = CertificateSummary("id", "Titular", "Emisor", "huella")
        override fun importCertificate(data: ByteArray, password: CharArray): CertificateSummary {
            imports++
            if (blockImport) throw UnsafeExceptionBlockedException(item())
            return selectCertificate()
        }
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput =
            throw UnsupportedOperationException()
        override fun sealPreview(certificateId: String, options: Map<String, String>) = byteArrayOf()
        override fun clearSession() = Unit
    }
}
