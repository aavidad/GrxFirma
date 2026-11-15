// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.net.FakeContentUri
import android.net.Uri
import io.github.aavidad.grxfirma.android.core.CoreBridge
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.core.UnsafeExceptionBlockedException
import io.github.aavidad.grxfirma.android.core.UnsafeExceptionState
import io.github.aavidad.grxfirma.android.core.UnsafeExceptions
import io.github.aavidad.grxfirma.android.files.DocumentRepository
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
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Lote 38: una verificación bloqueada por una excepción insegura abre el mismo
 * diálogo que el resto de operaciones, la tarjeta no la presenta como firma no
 * válida y, al cambiar el ajuste, no queda debajo el resultado anterior.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModelUnsafeVerificationTest {
    private val dispatcher = UnconfinedTestDispatcher()

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    private class Repository : DocumentRepository {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) = SelectedFile(uri, "firma.csig", "application/pkcs7-signature", 5)
        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(1, 2, 3, 4, 5))
        override fun loadCertificate(file: SelectedFile) = loadDocument(file)
        override fun write(uri: Uri, bytes: ByteArray) = Unit
    }

    private class Core : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "")
        override val unsafeExceptionsAvailable = true
        var enabled = false
        var activable = true
        var verifications = 0
        private fun item(id: String) = UnsafeExceptionState(id, enabled = enabled && id == "sha1_leer",
            fixed = !activable, activable = activable, acceptanceVersion = "core.version.2")
        override fun unsafeExceptions() = UnsafeExceptions.ids.map(::item)
        override fun activateUnsafeException(item: UnsafeExceptionState, accepted: Boolean) { enabled = true }
        override fun disableUnsafeException(item: UnsafeExceptionState): List<UnsafeExceptionState> {
            enabled = false
            return unsafeExceptions()
        }
        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary {
            verifications++
            // El núcleo entrega el bloqueo tipado; el puente lo convierte en esta excepción.
            if (!enabled) throw UnsafeExceptionBlockedException(item("sha1_leer").copy(acceptanceVersion = ""))
            return VerificationSummary(false, "", emptyList(), emptyList(), "CAdES", "full", "valid", "unknown", "unknown",
                "embedded_evidence_only", listOf("excepciones.sha1_leer.aviso"), emptyList())
        }
        override fun selectCertificate() = CertificateSummary("id", "Titular", "Emisor", "huella")
        override fun importCertificate(data: ByteArray, password: CharArray) = selectCertificate()
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput =
            throw UnsupportedOperationException()
        override fun sealPreview(certificateId: String, options: Map<String, String>) = byteArrayOf()
        override fun clearSession() = Unit
    }

    private fun ready(core: Core = Core()): Pair<MainViewModel, Core> {
        val vm = MainViewModel(Repository(), core, dispatcher)
        vm.selectDocument(FakeContentUri("content://test/firma"))
        return vm to core
    }

    @Test fun `a blocked verification opens the dialog and the card says it was not checked`() {
        val (vm, core) = ready()
        vm.verify()
        val state = vm.state.value
        assertNotNull(state.unsafePrompt)
        assertTrue(state.unsafePromptFromBlock)
        assertNull("no hay resultado de firma que enseñar", state.verification)
        assertEquals(OperationResult.Blocked(UiText.Engine("android.inseguro.verificacion_bloqueada"),
            UiText.Engine("excepciones.sha1_leer.bloqueo")), state.result)
        assertEquals("excepciones.sha1_leer.titulo", UnsafeExceptionDialog.titleKey(state.unsafePrompt!!, state.unsafePromptFromBlock))

        vm.activateUnsafeException(true)
        assertEquals("tras activar se verifica otra vez el mismo documento", 2, core.verifications)
        assertNull(vm.state.value.unsafePrompt)
        assertNotNull(vm.state.value.verification)
        assertTrue(vm.state.value.result is OperationResult.Success)
    }

    @Test fun `an organization block explains the policy instead of how to enable it`() {
        val (vm, core) = ready(Core().apply { activable = false })
        vm.verify()
        assertFalse(vm.state.value.unsafePrompt!!.canActivate)
        assertEquals(OperationResult.Blocked(UiText.Engine("android.inseguro.verificacion_bloqueada"),
            UiText.Engine("excepciones.sha1_leer.organizacion")), vm.state.value.result)
        assertEquals(1, core.verifications)
    }

    @Test fun `opening from preferences uses the neutral title`() {
        val (vm, _) = ready()
        val item = UnsafeExceptionState("sha1_leer", fixed = false, activable = true, acceptanceVersion = "core.version.2")
        vm.requestUnsafeActivation(item)
        assertFalse(vm.state.value.unsafePromptFromBlock)
        assertEquals("android.inseguro.aceptar_titulo", UnsafeExceptionDialog.titleKey(item, false))
        assertTrue(EngineKeys.isClosed("android.inseguro.aceptar_titulo"))
    }

    @Test fun `changing the setting from preferences clears the old result and names the setting`() {
        val (vm, core) = ready()
        core.enabled = true
        vm.verify()
        assertNotNull(vm.state.value.verification)
        core.enabled = false

        val item = UnsafeExceptionState("sha1_leer", fixed = false, activable = true, acceptanceVersion = "core.version.2")
        vm.requestUnsafeActivation(item)
        vm.activateUnsafeException(true)
        val setting = mapOf("ajuste" to UiText.Engine("excepciones.sha1_leer.ajuste"))
        assertNull("el resultado viejo no queda debajo", vm.state.value.verification)
        assertEquals(OperationResult.Notice(UiText.Engine("android.inseguro.activada_resultado", setting)), vm.state.value.result)

        vm.verify()
        assertNotNull(vm.state.value.verification)
        vm.disableUnsafeException(vm.state.value.unsafeExceptions.single { it.id == "sha1_leer" })
        assertNull(vm.state.value.verification)
        assertEquals(OperationResult.Notice(UiText.Engine("android.inseguro.desactivada_resultado", setting)), vm.state.value.result)
    }

    @Test fun `technical details show each text once`() {
        val block = "texto del bloqueo"
        val v = UiText.Verification(valid = false, reason = block, format = "CAdES", signerCount = 1, integrityStatus = "unknown",
            certificateStatus = "unknown", trustStatus = "unknown", revocationMode = "", warningCount = 2, errorCount = 1,
            warnings = listOf(block, "otro aviso", "otro aviso"), errors = listOf(block))
        val (warnings, errors) = VerificationCard.withoutRepeats(v, reasonShown = true)
        assertEquals(listOf("otro aviso"), warnings)
        assertEquals(emptyList<String>(), errors)
        val (onlyWarnings, onlyErrors) = VerificationCard.withoutRepeats(v, reasonShown = false)
        assertEquals(listOf(block), onlyErrors)
        assertEquals(listOf("otro aviso"), onlyWarnings)
    }
}
