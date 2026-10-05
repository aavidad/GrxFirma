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
import io.github.aavidad.grxfirma.android.core.PlatformServices
import io.github.aavidad.grxfirma.android.files.ContentRepository
import io.github.aavidad.grxfirma.android.model.CertificateDetail
import io.github.aavidad.grxfirma.android.model.CertificateDetails
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.model.SignedOutput
import io.github.aavidad.grxfirma.android.model.VeriFactuQr
import io.github.aavidad.grxfirma.android.model.VerificationSummary
import io.github.aavidad.grxfirma.android.settings.AppSettings
import io.github.aavidad.grxfirma.android.settings.InMemorySettingsStore
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

/** Varias identidades por sesión y lectura del QR desde una imagen, con un núcleo simulado. */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(AndroidJUnit4::class)
class MainViewModelIdentitiesTest {
    @After fun resetDispatcher() = Dispatchers.resetMain()

    @Test fun severalCertificatesStayOpenAndSigningUsesTheChosenOne() = runTest {
        val (vm, core) = ready()
        open(vm)
        open(vm)
        val state = vm.state.value
        assertEquals(listOf("id-1", "id-2"), state.identities.map { it.id })
        assertEquals("id-2", state.certificate?.id)
        assertTrue(state.showsIdentityList)
        assertEquals(2, state.certificateDetails.size)

        vm.selectIdentity("id-1")
        assertEquals("id-1", vm.state.value.certificate?.id)
        vm.sign("cades")
        advanceUntilIdle()
        assertEquals("id-1", core.lastSigningId)
        assertEquals(0, core.clearSessionCalls)
    }

    @Test fun closingOneKeepsTheOthersAndClosingAllClearsTheSession() = runTest {
        val (vm, core) = ready()
        open(vm)
        open(vm)
        open(vm)
        vm.closeIdentity("id-3")
        assertEquals(listOf("id-3"), core.removed)
        assertEquals(listOf("id-1", "id-2"), vm.state.value.identities.map { it.id })
        assertEquals("id-2", vm.state.value.certificate?.id) // el último que sigue abierto
        assertEquals(R.string.result_identity_closed,
            ((vm.state.value.result as OperationResult.Success).title as UiText.Resource).id)

        vm.forgetCertificate()
        assertEquals(1, core.clearSessionCalls)
        assertTrue(vm.state.value.identities.isEmpty())
        assertNull(vm.state.value.certificate)
        assertEquals(OperationResult.Success(UiText.Resource(R.string.result_certificates_forgotten)), vm.state.value.result)
    }

    @Test fun backgroundTimeoutClosesEveryCertificate() = runTest {
        val (vm, core) = ready(InMemorySettingsStore(AppSettings(sessionTimeoutMinutes = 1)))
        open(vm)
        open(vm)
        assertEquals(AutoClose.NOT_DUE, vm.closeCertificateAfterBackground(30_000))
        assertEquals(AutoClose.CLOSED, vm.closeCertificateAfterBackground(61_000))
        assertEquals(1, core.clearSessionCalls)
        assertTrue(vm.state.value.identities.isEmpty())
        assertEquals(OperationResult.Notice(UiText.Resource(R.string.certificate_closed_timeout)), vm.state.value.result)
    }

    @Test fun reachingTheLimitShowsAPlainMessage() = runTest {
        val (vm, core) = ready()
        open(vm)
        core.full = true
        open(vm)
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_session_full)), vm.state.value.result)
        assertEquals(1, vm.state.value.identities.size)
    }

    @Test fun anOldCoreKeepsReplacingTheCertificate() = runTest {
        val (vm, core) = ready(core = FakeCore(maxIdentities = 1))
        open(vm)
        open(vm)
        assertEquals(listOf("id-2"), vm.state.value.identities.map { it.id })
        assertFalse(vm.state.value.showsIdentityList)
        vm.closeIdentity("id-2")
        assertEquals(1, core.clearSessionCalls)
        assertTrue(core.removed.isEmpty())
    }

    @Test fun qrImageIsReadWipedAndTheTemporaryFileRemoved() = runTest {
        val (vm, core) = ready()
        var prepared: ByteArray? = null
        var finished = false
        vm.readVeriFactuQrImage(IMAGE_URI, prepare = { source -> source.copyOf().also { prepared = it } }) { finished = true }
        advanceUntilIdle()
        assertEquals("89890001K", vm.state.value.veriFactuQr?.nif)
        assertTrue(finished)
        assertArrayEquals(ByteArray(IMAGE.size), core.lastImage)
        assertArrayEquals(ByteArray(IMAGE.size), prepared)
        assertTrue(vm.state.value.canQueryAeat)

        core.qrError = "verifactu.qr_not_found"
        vm.readVeriFactuQrImage(IMAGE_URI)
        advanceUntilIdle()
        assertNull(vm.state.value.veriFactuQr)
        assertEquals(UiText.Resource(R.string.qr_error_not_found), vm.state.value.qrError)
    }

    @Test fun qrImageIsRefusedWithoutTheServiceButStillCleansUp() = runTest {
        val (vm, _) = ready(core = FakeCore(services = emptySet()))
        var finished = false
        vm.readVeriFactuQrImage(IMAGE_URI) { finished = true }
        advanceUntilIdle()
        assertTrue(finished)
        assertNull(vm.state.value.veriFactuQr)
    }

    private fun TestScope.ready(
        settings: InMemorySettingsStore = InMemorySettingsStore(),
        core: FakeCore = FakeCore(),
    ): Pair<MainViewModel, FakeCore> {
        Dispatchers.setMain(StandardTestDispatcher(testScheduler))
        val vm = MainViewModel(FakeRepository(), core, StandardTestDispatcher(testScheduler), settings)
        vm.selectDocument(DOCUMENT_URI)
        advanceUntilIdle()
        return vm to core
    }

    private fun TestScope.open(vm: MainViewModel) {
        vm.selectCertificateFile(CERTIFICATE_URI)
        vm.importCertificate(charArrayOf('p'))
        advanceUntilIdle()
    }

    private class FakeRepository : ContentRepository(ApplicationProvider.getApplicationContext<Context>().contentResolver) {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String) =
            SelectedFile(uri, fallbackName, fallbackMime, 1)
        override fun loadDocument(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(9))
        override fun loadBounded(file: SelectedFile, maximumBytes: Int) = LoadedFile(file.displayName, file.mimeType, IMAGE.copyOf())
        override fun loadCertificate(file: SelectedFile) = LoadedFile(file.displayName, file.mimeType, byteArrayOf(8, 8))
        override fun write(uri: Uri, bytes: ByteArray) = Unit
    }

    private class FakeCore(
        override val maxIdentities: Int = 8,
        services: Set<String> = setOf(PlatformServices.CERTIFICATE_DETAILS, PlatformServices.SESSION_IDENTITIES,
            PlatformServices.VERIFACTU_QR_IMAGE, PlatformServices.VERIFACTU_QR_QUERY),
    ) : CoreBridge {
        override val readiness = CoreReadiness(true, "ready", "Disponible")
        override val platformServices: Set<String> = services
        private val open = mutableListOf<String>()
        private var next = 0
        var clearSessionCalls = 0
        val removed = mutableListOf<String>()
        var lastSigningId: String? = null
        var full = false
        var lastImage: ByteArray? = null
        var qrError: String? = null

        override fun selectCertificate() = error("no")
        override fun importCertificate(data: ByteArray, password: CharArray): CertificateSummary {
            password.fill('\u0000')
            if (full) throw CoreContractException("session.full")
            val id = "id-${++next}"
            if (maxIdentities <= 1) open.clear()
            open += id
            return CertificateSummary(id, "Titular $id", "Emisor", id)
        }
        override fun sign(document: LoadedFile, format: String, certificateId: String, options: Map<String, String>, action: String): SignedOutput {
            lastSigningId = certificateId
            return SignedOutput(byteArrayOf(1), "firmado.p7s", "application/pkcs7-signature", "CAdES", "SHA256withRSA")
        }
        override fun sealPreview(certificateId: String, options: Map<String, String>) = byteArrayOf()
        override fun verify(document: LoadedFile, original: LoadedFile?) = VerificationSummary(true, "", emptyList(),
            emptyList(), "CAdES", "full", "valid", "valid", "unknown", "embedded_evidence_only", emptyList(), emptyList())
        override fun clearSession() { clearSessionCalls++; open.clear() }
        override fun removeIdentity(certificateId: String): Int {
            removed += certificateId
            open.remove(certificateId)
            return open.size
        }
        override fun certificateDetails() = CertificateDetails(30, open.map { id ->
            CertificateDetail(id, "Titular $id", "CA", id, "", "", "fisica", "RSA", 2048,
                "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z", 400, "valid", false, true, true, false)
        })
        override fun readVeriFactuQrImage(image: ByteArray): VeriFactuQr {
            lastImage = image
            try {
                qrError?.let { throw CoreContractException(it) }
                return VeriFactuQr(QR_URL, "89890001K", "A1", "01-01-2025", "10.00", true, false)
            } finally {
                image.fill(0)
            }
        }
    }

    private companion object {
        val DOCUMENT_URI: Uri = Uri.parse("content://test/document")
        val CERTIFICATE_URI: Uri = Uri.parse("content://test/certificate")
        val IMAGE_URI: Uri = Uri.parse("content://test/qr.jpg")
        val IMAGE = byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 0xFF.toByte(), 1, 2, 3)
        const val QR_URL = "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=A1&fecha=01-01-2025&importe=10.00"
    }
}
