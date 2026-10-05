// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.net.Uri
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreBridge
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.files.DocumentRepository
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
import io.github.aavidad.grxfirma.android.model.VerificationSummary
import io.github.aavidad.grxfirma.android.settings.AppSettings
import io.github.aavidad.grxfirma.android.settings.InMemorySettingsStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class UsabilityReviewTest {
    private val dispatcher = UnconfinedTestDispatcher()
    private val repository = object : DocumentRepository {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile = error("Unexpected I/O")
        override fun loadDocument(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun loadCertificate(file: SelectedFile): LoadedFile =
            LoadedFile("c.p12", "application/x-pkcs12", byteArrayOf(1, 2, 3))
        override fun write(uri: Uri, bytes: ByteArray): Unit = error("Unexpected I/O")
    }

    private class FakeCore : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "")
        var cleared = 0
        override fun selectCertificate(): CertificateSummary = error("no")
        override fun importCertificate(data: ByteArray, password: CharArray) = CertificateSummary("id", "Ana", "CA", "ff")
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput = error("no")
        val previewOptions = mutableListOf<Map<String, String>>()
        override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray {
            previewOptions += options
            return byteArrayOf(1)
        }
        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary = error("no")
        override fun clearSession() { cleared++ }
        val regions = mutableListOf<Pair<String, String>>()
        override fun setRegion(language: String, timeZone: String) {
            regions += language to timeZone
            if (timeZone == "Marte/Olimpo") throw io.github.aavidad.grxfirma.android.core.CoreContractException("zona horaria no reconocida")
        }
    }

    @Test fun `region reaches the core once per change and keeps the language with an unknown zone`() {
        val core = FakeCore()
        val vm = MainViewModel(repository, core, dispatcher, InMemorySettingsStore())
        vm.updateRegion("ca-ES-valencia", "Europe/Madrid")
        vm.updateRegion("ca-ES-valencia", "Europe/Madrid")
        assertEquals(listOf("ca-ES-valencia" to "Europe/Madrid"), core.regions)
        vm.updateRegion("en", "Marte/Olimpo")
        assertEquals(listOf("ca-ES-valencia" to "Europe/Madrid", "en" to "Marte/Olimpo", "en" to ""), core.regions)
    }

    @Test fun `fixed seal language travels with the seal options`() = kotlinx.coroutines.test.runTest(dispatcher) {
        val core = FakeCore()
        val vm = withCertificate(core, 5, AppSettings(sealLanguage = "es"))
        vm.previewSeal(mapOf("visibleSeal" to "true"))
        assertEquals("es", core.previewOptions.last()["sealLanguage"])
        val following = withCertificate(core, 5, AppSettings())
        following.previewSeal(mapOf("visibleSeal" to "true"))
        assertNull(core.previewOptions.last()["sealLanguage"])
    }

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    private fun withCertificate(core: FakeCore, minutes: Int, settings: AppSettings = AppSettings()): MainViewModel {
        val vm = MainViewModel(repository, core, dispatcher, InMemorySettingsStore(settings.copy(sessionTimeoutMinutes = minutes)))
        val field = MainViewModel::class.java.getDeclaredField("mutableState").apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val state = field.get(vm) as kotlinx.coroutines.flow.MutableStateFlow<MainUiState>
        state.value = state.value.copy(certificate = CertificateSummary("id", "Ana", "CA", "ff"))
        return vm
    }

    @Test fun `PKCS12 closes after the chosen time in the background with a neutral notice`() {
        val core = FakeCore()
        val vm = withCertificate(core, 5)
        assertEquals(AutoClose.NOT_DUE, vm.closeCertificateAfterBackground(4 * 60_000L))
        assertNotNull(vm.state.value.certificate)
        assertEquals(AutoClose.CLOSED, vm.closeCertificateAfterBackground(5 * 60_000L))
        assertNull(vm.state.value.certificate)
        assertEquals(1, core.cleared)
        assertEquals(OperationResult.Notice(UiText.Resource(R.string.certificate_closed_timeout)), vm.state.value.result)
    }

    @Test fun `zero minutes never closes the certificate by itself`() {
        val vm = withCertificate(FakeCore(), 0)
        assertEquals(AutoClose.NOT_APPLICABLE, vm.closeCertificateAfterBackground(24 * 3_600_000L))
        assertNotNull(vm.state.value.certificate)
    }

    @Test fun `an operation in progress or an unsaved result postpones the close instead of dropping it`() {
        val core = FakeCore()
        val vm = withCertificate(core, 5)
        val field = MainViewModel::class.java.getDeclaredField("mutableState").apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val state = field.get(vm) as kotlinx.coroutines.flow.MutableStateFlow<MainUiState>
        state.value = state.value.copy(busy = true)
        assertEquals(AutoClose.POSTPONED, vm.closeCertificateAfterBackground(10 * 60_000L))
        state.value = state.value.copy(busy = false, awaitingSave = true)
        assertEquals(AutoClose.POSTPONED, vm.closeCertificateAfterBackground(10 * 60_000L))
        assertNotNull(vm.state.value.certificate)
        assertEquals(0, core.cleared)
        state.value = state.value.copy(awaitingSave = false)
        assertEquals(AutoClose.CLOSED, vm.closeCertificateAfterBackground(10 * 60_000L))
        assertNull(vm.state.value.certificate)
    }

    @Test fun `certified date and time keep the signature profile coherent`() {
        val vm = MainViewModel(repository, FakeCore(), dispatcher,
            InMemorySettingsStore(AppSettings(tsaUrl = "https://tsa.example/tsr")))
        vm.updateSigningSettings("sign", "baseline", true, "")
        assertEquals("t", vm.state.value.signatureProfile)
        assertEquals("se propone el servicio de Preferencias", "https://tsa.example/tsr", vm.state.value.tsaUrl)
        vm.updateSigningSettings("sign", "lta", true, "https://tsa.example/tsr")
        assertEquals("lta", vm.state.value.signatureProfile)
        vm.updateSigningSettings("sign", "lta", false, "https://tsa.example/tsr")
        assertEquals("baseline", vm.state.value.signatureProfile)
        vm.updateSigningSettings("sign", "lt", false, "https://tsa.example/tsr")
        assertEquals(true, vm.state.value.tsaEnabled)
        vm.updateSigningSettings("sign", "baseline", true, "https://tsa.example/tsr")
        assertEquals(false, vm.state.value.tsaEnabled)
    }

    @Test fun `timeout setting falls back to five minutes`() {
        assertEquals(5, AppSettings(sessionTimeoutMinutes = 7).sanitized().sessionTimeoutMinutes)
        assertEquals(0, AppSettings(sessionTimeoutMinutes = 0).sanitized().sessionTimeoutMinutes)
    }

    @Test fun `language selector marks the active language even with a region`() {
        val tags = listOf("", "es", "en", "ca", "ca-ES-valencia", "gl", "eu", "fr", "de", "it", "pt", "zh")
        assertEquals(0, LanguageTags.indexFor("", tags))
        assertEquals(1, LanguageTags.indexFor("es-ES", tags))
        assertEquals(2, LanguageTags.indexFor("en-GB,es", tags))
        assertEquals(4, LanguageTags.indexFor("ca-ES-valencia", tags))
        assertEquals(3, LanguageTags.indexFor("ca-ES", tags))
        assertEquals(11, LanguageTags.indexFor("zh-Hans-CN", tags))
        assertEquals(0, LanguageTags.indexFor("ja-JP", tags))
    }
}
