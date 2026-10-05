// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import android.net.Uri
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.core.DocumentServices
import io.github.aavidad.grxfirma.android.core.UnavailableCoreBridge
import io.github.aavidad.grxfirma.android.core.Wave4Capabilities
import io.github.aavidad.grxfirma.android.files.DocumentRepository
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.model.EniFileRequest
import io.github.aavidad.grxfirma.android.model.LoadedFile
import io.github.aavidad.grxfirma.android.model.SelectedFile
import org.junit.Assert.*
import org.junit.Test

class Wave4PolicyTest {
    @Test fun `folder keeps XML documents sorted by name and counts the rest`() {
        val entries = listOf("b.xml" to "application/octet-stream", "nota.txt" to "text/plain",
            "a.dat" to "application/xml", "C.XML" to "application/octet-stream")
        assertEquals(listOf(3, 2, 0), ExpedientePolicy.xmlOrder(entries))
    }

    @Test fun `selection limits match the mobile core`() {
        assertEquals(R.string.expediente_error_no_xml, ExpedientePolicy.selectionProblem(emptyList()))
        assertEquals(R.string.expediente_error_too_many,
            ExpedientePolicy.selectionProblem(List(ExpedientePolicy.MAX_DOCUMENTS + 1) { 10L }))
        assertEquals(R.string.expediente_error_too_large,
            ExpedientePolicy.selectionProblem(listOf(20L shl 20, 20L shl 20)))
        assertEquals(R.string.expediente_error_too_large, ExpedientePolicy.selectionProblem(listOf(33L shl 20)))
        assertNull(ExpedientePolicy.selectionProblem(listOf(10L, null)))
    }

    @Test fun `metadata rules follow the ENI engine`() {
        assertTrue(ExpedientePolicy.classificationValid("1234"))
        assertTrue(ExpedientePolicy.classificationValid("L01180877_PRO_000001"))
        assertFalse(ExpedientePolicy.classificationValid(""))
        assertFalse(ExpedientePolicy.classificationValid("granada"))
        assertTrue(ExpedientePolicy.identifierValid(""))
        assertTrue(ExpedientePolicy.identifierValid("ES_L01180877_2026_EXP_1"))
        assertFalse(ExpedientePolicy.identifierValid("expediente 1"))
        assertEquals(listOf("Ana López, S.L.", "Luis"), ExpedientePolicy.interested(" Ana López, S.L. ;\nLuis;; Luis"))
        assertFalse(ExpedientePolicy.interestedValid(List(17) { "p$it" }))
        assertFalse(ExpedientePolicy.interestedValid(listOf("a".repeat(129))))
        assertEquals("expediente.eni.xml", ExpedientePolicy.outputName(""))
        assertEquals("ES_L01180877_2026_X.eni.xml", ExpedientePolicy.outputName("ES_L01180877_2026_X"))
    }

    @Test fun `DNIe batch and protect and sign follow the declared capabilities`() {
        val certificate = CertificateSummary("id", "Persona", "CA", "ff")
        val ready = MainUiState(CoreReadiness(true, "ready", ""), certificate = certificate, toolsAvailable = true,
            certificateExternal = true)
        assertFalse(ready.externalBatchAvailable)
        assertFalse(ready.externalProtectSignAvailable)
        val declared = ready.copy(capabilities = setOf(Wave4Capabilities.EXTERNAL_BATCH,
            Wave4Capabilities.EXTERNAL_PROTECT_SIGN, Wave4Capabilities.BATCH_COSIGN))
        assertTrue(declared.externalBatchAvailable)
        assertTrue(declared.externalProtectSignAvailable)
        assertTrue(declared.batchCosignAvailable)
        assertFalse(declared.batchSealAvailable)
        assertFalse("el certificado de FIRMA del DNIe no cifra", declared.canProtectForMe)
        assertFalse("sin ficheros no hay lote", declared.canSignBatch)
    }

    @Test fun `ENI file is offered only when the core declares it`() {
        val base = MainUiState(CoreReadiness(true, "ready", ""), documentServices = setOf(DocumentServices.ENI_FILE),
            certificate = CertificateSummary("id", "P", "CA", "ff"))
        assertTrue(base.eniFileAvailable)
        assertFalse("sin documentos no se puede crear", base.canCreateEniFile)
        assertFalse(base.copy(documentServices = emptySet()).eniFileAvailable)
        assertFalse(base.copy(backend = CoreReadiness(false, "x", "")).eniFileAvailable)
    }

    private val repository = object : DocumentRepository {
        override fun inspect(uri: Uri, fallbackName: String, fallbackMime: String): SelectedFile = error("Unexpected I/O")
        override fun loadDocument(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun loadCertificate(file: SelectedFile): LoadedFile = error("Unexpected I/O")
        override fun write(uri: Uri, bytes: ByteArray): Unit = error("Unexpected I/O")
    }

    @Test fun `view model rejects incomplete requests before any signature`() {
        val vm = MainViewModel(repository, UnavailableCoreBridge(CoreReadiness(false, "verification_build", "")))
        var finished = 0
        vm.createEniFile(EniFileRequest(listOf("L01180877"), "123", "E01")) { finished++ }
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_core_unavailable)), vm.state.value.result)
        vm.signBatch("auto", onFinished = { finished++ })
        assertEquals(OperationResult.Error(UiText.Resource(R.string.error_batch_required)), vm.state.value.result)
        assertEquals("cerrar la operación de PIN también si no empieza", 2, finished)
        vm.updateBatchOptions("cosign", true)
        assertEquals("cosign", vm.state.value.wave4.batchAction)
        assertTrue(vm.state.value.wave4.batchSeal)
        assertThrows(IllegalArgumentException::class.java) { vm.updateBatchOptions("countersign", false) }
    }
}
