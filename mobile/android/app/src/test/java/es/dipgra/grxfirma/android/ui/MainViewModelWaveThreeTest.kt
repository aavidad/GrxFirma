// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import android.net.Uri
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.core.CoreBridge
import es.dipgra.grxfirma.android.core.CoreContractException
import es.dipgra.grxfirma.android.core.CoreReadiness
import es.dipgra.grxfirma.android.core.PlatformServices
import es.dipgra.grxfirma.android.files.DocumentRepository
import es.dipgra.grxfirma.android.model.CertificateDetail
import es.dipgra.grxfirma.android.model.CertificateDetails
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.EngineDiagnostics
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.RevocationCheck
import es.dipgra.grxfirma.android.model.SelectedFile
import es.dipgra.grxfirma.android.model.SignedOutput
import es.dipgra.grxfirma.android.model.TsaProbe
import es.dipgra.grxfirma.android.model.UpdateCheck
import es.dipgra.grxfirma.android.model.VeriFactuQr
import es.dipgra.grxfirma.android.model.VerificationSummary
import es.dipgra.grxfirma.android.settings.AppSettings
import es.dipgra.grxfirma.android.settings.InMemorySettingsStore
import es.dipgra.grxfirma.android.settings.OutputNames
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

@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModelWaveThreeTest {
    private val dispatcher = UnconfinedTestDispatcher()
    private val repository = object : DocumentRepository {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile = error("Unexpected I/O")
        override fun loadDocument(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun loadCertificate(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun write(uri: Uri, bytes: ByteArray): Unit = error("Unexpected I/O")
    }

    private class FakeCore(override val platformServices: Set<String> = PlatformServices.ALL.toSet()) : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "")
        val queried = mutableListOf<String>()
        var revocation = RevocationCheck("revoked", "OCSP", "2026-10-01T10:00:00Z", "2026-09-01T10:00:00Z", true, false)
        override fun selectCertificate(): CertificateSummary = error("no")
        override fun importCertificate(data: ByteArray, password: CharArray) = error("no")
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput = error("no")
        override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray = error("no")
        override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary = error("no")
        override fun clearSession() = Unit
        override fun certificateDetails() = CertificateDetails(30, listOf(detail("id", "fisica", "12345678Z", "Diputación")))
        override fun checkCertificateRevocation(certificateId: String) = revocation
        override fun diagnostics() = EngineDiagnostics("0.0.116", 2, "android", "go1.26", "android/arm64", "2026-10-05T10:00:00Z", true)
        override fun probeTimestampAuthority(url: String) = TsaProbe("ok", true, "2026-10-05T10:00:00Z", "2026-10-05T10:00:03Z", 3, 120)
        override fun readVeriFactuQr(url: String): VeriFactuQr {
            if (!url.startsWith("https://www2.agenciatributaria.gob.es/")) throw CoreContractException("verifactu.qr_url")
            return VeriFactuQr(url, "89890001K", "A1", "01-01-2025", "10.00", true, false)
        }
        override fun queryVeriFactuQr(url: String): String { queried += url; return "{\n  \"ok\": true\n}" }
        override fun checkUpdate(currentVersion: String) =
            UpdateCheck("newer", "", currentVersion, "v9.0.0", "https://github.com/aavidad/GrxFirma/releases/tag/v9.0.0")
    }

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    private fun model(core: FakeCore = FakeCore(), settings: InMemorySettingsStore = InMemorySettingsStore()) =
        MainViewModel(repository, core, dispatcher, settings)

    @Test fun `initial session takes the saved defaults`() {
        val settings = InMemorySettingsStore(AppSettings(defaultProfile = "t", tsaEnabled = true, tsaUrl = "https://tsa.example"))
        val state = model(settings = settings).state.value
        assertEquals("t", state.signatureProfile)
        assertTrue(state.tsaEnabled)
        assertEquals("https://tsa.example", state.tsaUrl)
        assertEquals(PlatformServices.ALL.toSet(), state.platformServices)
    }

    @Test fun `saving rejects a timestamp profile without a valid TSA`() {
        val settings = InMemorySettingsStore()
        val vm = model(settings = settings)
        assertFalse(vm.savePreferences(AppSettings(defaultProfile = "lt", tsaEnabled = false)))
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_tsa_configuration)), vm.state.value.result)
        assertFalse(vm.savePreferences(AppSettings(tsaEnabled = true, tsaUrl = "https://user:pw@tsa.example")))
        assertEquals(AppSettings(), settings.load())

        assertTrue(vm.savePreferences(AppSettings(defaultFormat = "cades", defaultProfile = "t", tsaEnabled = true,
            tsaUrl = "https://tsa.example/tsr", outputName = OutputNames.DESKTOP, theme = AppSettings.THEME_DARK)))
        assertEquals("cades", settings.load().defaultFormat)
        assertEquals("t", vm.state.value.signatureProfile)
        assertEquals("https://tsa.example/tsr", vm.state.value.tsaUrl)
    }

    @Test fun `restoring defaults resets the store and the session settings`() {
        val settings = InMemorySettingsStore(AppSettings(defaultProfile = "t", tsaEnabled = true, tsaUrl = "https://tsa.example"))
        val vm = model(settings = settings)
        vm.restoreDefaultPreferences()
        assertEquals(1, settings.resets)
        assertEquals("baseline", vm.state.value.signatureProfile)
        assertFalse(vm.state.value.tsaEnabled)
        assertEquals(OperationResult.Success(UiText.Resource(R.string.preferences_restored)), vm.state.value.result)
    }

    @Test fun `online check needs a certificate and reports revocation as an error`() {
        val vm = model()
        vm.checkCertificateOnline()
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_certificate_required)), vm.state.value.result)
        val state = MainUiState(CoreReadiness(true, "ready", ""), platformServices = PlatformServices.ALL.toSet(),
            certificate = CertificateSummary("id", "Persona", "CA", "ff"))
        assertTrue(state.canCheckCertificateOnline)
        assertFalse(state.copy(platformServices = emptySet()).canCheckCertificateOnline)
    }

    @Test fun `QR must be read before the AEAT is queried`() {
        val core = FakeCore()
        val vm = model(core)
        vm.queryAeat()
        assertTrue(core.queried.isEmpty())
        assertFalse(vm.state.value.canQueryAeat)

        vm.readVeriFactuQr("https://evil.test/x")
        assertNull(vm.state.value.veriFactuQr)
        assertEquals(UiText.Engine("verifactu.qr_url"), vm.state.value.qrError)

        val url = "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=A1&fecha=01-01-2025&importe=10.00"
        vm.readVeriFactuQr("  $url  ")
        assertNotNull(vm.state.value.veriFactuQr)
        assertTrue(vm.state.value.canQueryAeat)
        vm.queryAeat()
        assertEquals(listOf(url), core.queried)
        assertTrue(vm.state.value.aeatResponse.contains("\"ok\""))

        vm.clearVeriFactuQr()
        assertFalse(vm.state.value.canQueryAeat)
    }

    @Test fun `diagnostics, TSA probe and update check fill the state`() {
        val vm = model(settings = InMemorySettingsStore(AppSettings(tsaEnabled = true, tsaUrl = "https://tsa.example")))
        vm.loadDiagnostics()
        assertEquals("0.0.116", vm.state.value.diagnostics?.engineVersion)
        vm.probeTsa()
        assertEquals(3L, vm.state.value.tsaProbe?.skewSeconds)
        vm.checkUpdate("0.0.115")
        assertEquals("newer", vm.state.value.updateCheck?.status)
        vm.dismissUpdateCheck()
        assertNull(vm.state.value.updateCheck)
    }

    @Test fun `old cores hide every third wave feature`() {
        val vm = model(FakeCore(platformServices = emptySet()))
        val state = vm.state.value
        assertFalse(state.certificatePanelAvailable)
        assertFalse(state.diagnosticsAvailable)
        assertFalse(state.qrReadAvailable)
        assertFalse(state.updateCheckAvailable)
        vm.checkUpdate("0.0.115")
        assertNull(vm.state.value.updateCheck)
    }

    @Test fun `certificate filter matches tax id, organization and kind`() {
        val list = listOf(detail("a", "fisica", "IDCES-12345678Z", "Diputación de Granada"),
            detail("b", "sello", "Q1800000A", "Ayuntamiento"))
        assertEquals(listOf("a"), CertificateFilter.apply(list, "12345678-z", "").map { it.id })
        assertEquals(listOf("a"), CertificateFilter.apply(list, "diputacion", "").map { it.id })
        assertEquals(listOf("b"), CertificateFilter.apply(list, "", "sello").map { it.id })
        assertTrue(CertificateFilter.apply(list, "granada", "sello").isEmpty())
        assertEquals(2, CertificateFilter.apply(list, "", "").size)
        val five = list + (1..3).map { detail("x$it", "fisica", "", "Otra") }
        val state = MainUiState(CoreReadiness(true, "ready", ""), certificateDetails = five, certificateFilter = "ayunt")
        assertTrue(state.showsCertificateFilter)
        assertEquals(listOf("b"), state.filteredCertificates.map { it.id })
        // Con menos de cinco no hay buscador ni filtro aplicado.
        val few = state.copy(certificateDetails = list)
        assertFalse(few.showsCertificateFilter)
        assertEquals(listOf("a", "b"), few.filteredCertificates.map { it.id })
    }

    companion object {
        fun detail(id: String, kind: String, nif: String, organization: String, status: String = "valid", days: Int = 400) =
            CertificateDetail(id, "Titular $id", "CA", "ff", nif, organization, kind, "RSA", 2048,
                "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z", days, status, false, true, true, false)
    }
}
