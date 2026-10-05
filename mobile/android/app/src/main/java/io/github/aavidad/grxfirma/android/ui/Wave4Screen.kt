// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android.ui

import android.view.View
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import com.google.android.material.datepicker.CalendarConstraints
import com.google.android.material.datepicker.DateValidatorPointBackward
import com.google.android.material.datepicker.MaterialDatePicker
import io.github.aavidad.grxfirma.android.R
import io.github.aavidad.grxfirma.android.databinding.SectionBatchWave4Binding
import io.github.aavidad.grxfirma.android.databinding.SectionExpedienteBinding
import io.github.aavidad.grxfirma.android.model.EniFileRequest
import java.text.DateFormat
import java.util.Date
import java.util.TimeZone

private const val EXPEDIENTE_DATE_PICKER = "eni_file_opening_date"

/**
 * Pantallas de la cuarta oleada (expediente ENI y opciones del lote). La
 * actividad le da acceso al DNIe mediante [dnie]; así MainActivity solo
 * delega y no crece con cada sección.
 */
internal class Wave4Screen(
    private val activity: AppCompatActivity,
    private val viewModel: MainViewModel,
    private val dnie: DnieAccess,
) {
    /** Lo que esta pantalla necesita del DNIe de la sesión, si lo hay. */
    interface DnieAccess {
        /** Pide el PIN (una vez si [hold]) y ejecuta [action]; sin DNIe la ejecuta directamente. */
        fun withPin(hold: Boolean, action: () -> Unit)
        fun endOperation()
        fun takeError(): Pair<Throwable, Int>?
    }

    private var expediente: SectionExpedienteBinding? = null
    private var batch: SectionBatchWave4Binding? = null
    private var expanded = false
    private var updating = false

    private val chooseFolder = activity.registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        if (uri != null) viewModel.selectEniFileFolder(uri)
    }

    fun bind(expediente: SectionExpedienteBinding, batch: SectionBatchWave4Binding, expandedInitially: Boolean) {
        this.expediente = expediente
        this.batch = batch
        expanded = expandedInitially
        configureExpediente(expediente)
        configureBatch(batch)
    }

    val isExpanded: Boolean get() = expanded

    private fun configureExpediente(view: SectionExpedienteBinding) = with(view) {
        toggleExpedienteButton.setOnClickListener {
            expanded = !expanded
            renderToggle(this)
        }
        expedienteState.setItems(ExpedientePolicy.STATES.map(::codeLabel))
        expedienteFolderButton.setOnClickListener {
            try { chooseFolder.launch(null) } catch (_: RuntimeException) { viewModel.reportPickerError() }
        }
        expedienteClearButton.setOnClickListener { viewModel.clearEniFileDocuments() }
        expedienteOpeningButton.setOnClickListener { showOpeningDatePicker() }
        expedienteOpeningClearButton.setOnClickListener { viewModel.updateEniFileOpeningDate(null) }
        expedienteCreateButton.setOnClickListener { createExpediente(this) }
        renderToggle(this)
    }

    private fun createExpediente(view: SectionExpedienteBinding) = with(view) {
        val organs = EniForm.organs(expedienteOrgans.text?.toString().orEmpty())
        val classification = expedienteClassification.text?.toString()?.trim().orEmpty()
        val identifier = expedienteIdentifier.text?.toString()?.trim().orEmpty()
        val interested = ExpedientePolicy.interested(expedienteInterested.text?.toString().orEmpty())
        expedienteOrgansLayout.error = if (EniForm.organsValid(organs)) null else
            EngineText.resolve(activity, "eni.validacion.dir3")
        expedienteClassificationLayout.error = if (ExpedientePolicy.classificationValid(classification)) null else
            EngineText.resolve(activity, "eni.validacion.classification")
        expedienteIdentifierLayout.error = if (ExpedientePolicy.identifierValid(identifier)) null else
            EngineText.resolve(activity, "eni.validacion.identifier")
        expedienteInterestedLayout.error = if (ExpedientePolicy.interestedValid(interested)) null else
            EngineText.resolve(activity, "eni.validacion.text")
        if (listOf(expedienteOrgansLayout, expedienteClassificationLayout, expedienteIdentifierLayout,
                expedienteInterestedLayout).any { it.error != null }) return@with
        val request = EniFileRequest(
            organs = organs,
            classification = classification,
            state = ExpedientePolicy.STATES.getOrElse(expedienteState.selectedItemPosition) { "E01" },
            identifier = identifier,
            openingDate = EniForm.captureDate(viewModel.state.value.wave4.eniFileOpeningDate),
            interested = interested,
        )
        // El índice lleva una sola firma: con DNIe, el PIN se borra tras ella.
        dnie.withPin(hold = false) { viewModel.createEniFile(request) }
    }

    private fun configureBatch(view: SectionBatchWave4Binding) = with(view) {
        batchAction.setItems(listOf(activity.getString(R.string.batch_action_sign), activity.getString(R.string.batch_action_cosign)))
        batchAction.onItemSelected = { update() }
        batchSealCheck.setOnCheckedChangeListener { _, _ -> update() }
    }

    private fun update() {
        val view = batch ?: return
        if (updating) return
        viewModel.updateBatchOptions(if (view.batchAction.selectedItemPosition == 1) "cosign" else "sign",
            view.batchSealCheck.isChecked)
    }

    /** Firma el lote; con DNIe pide el PIN una vez y lo borra al terminar. */
    fun signBatch(format: String, sealPlanner: BatchSealPlanner?) {
        dnie.withPin(hold = true) {
            viewModel.signBatch(format, sealPlanner, externalError = dnie::takeError, onFinished = dnie::endOperation)
        }
    }

    fun render(state: MainUiState) {
        expediente?.let { renderExpediente(it, state) }
        batch?.let { renderBatch(it, state) }
    }

    private fun renderExpediente(view: SectionExpedienteBinding, state: MainUiState) = with(view) {
        val available = state.eniFileAvailable
        toggleExpedienteButton.visibility = if (available) View.VISIBLE else View.GONE
        if (!available) expedienteGroup.visibility = View.GONE else renderToggle(this)
        val idle = state.canReplaceSelection
        val documents = state.wave4.eniFileDocuments
        expedienteSummary.text = if (documents.isEmpty()) activity.getString(R.string.expediente_none) else buildString {
            append(activity.resources.getQuantityString(R.plurals.expediente_documents_count, documents.size, documents.size))
            if (state.wave4.eniFileSkipped > 0) {
                append('\n').append(activity.resources.getQuantityString(R.plurals.expediente_skipped_count,
                    state.wave4.eniFileSkipped, state.wave4.eniFileSkipped))
            }
            documents.forEach { append('\n').append(it.displayName) }
        }
        expedienteFolderButton.isEnabled = available && idle
        expedienteClearButton.visibility = if (documents.isEmpty()) View.GONE else View.VISIBLE
        expedienteClearButton.isEnabled = idle
        listOf(expedienteOrgansLayout, expedienteClassificationLayout, expedienteIdentifierLayout,
            expedienteInterestedLayout).forEach { it.isEnabled = idle }
        expedienteState.isEnabled = idle
        expedienteOpeningSummary.text = state.wave4.eniFileOpeningDate?.let {
            val format = DateFormat.getDateInstance(DateFormat.LONG).apply { timeZone = TimeZone.getTimeZone("UTC") }
            activity.getString(R.string.expediente_opening_value, format.format(Date(it)))
        } ?: activity.getString(R.string.expediente_opening_now)
        expedienteOpeningButton.isEnabled = idle
        expedienteOpeningClearButton.visibility = if (state.wave4.eniFileOpeningDate == null) View.GONE else View.VISIBLE
        expedienteOpeningClearButton.isEnabled = idle
        expedienteCreateButton.isEnabled = state.canCreateEniFile
    }

    private fun renderBatch(view: SectionBatchWave4Binding, state: MainUiState) = with(view) {
        val idle = state.canReplaceSelection
        val cosign = state.batchCosignAvailable
        batchActionLayout.visibility = if (cosign) View.VISIBLE else View.GONE
        batchAction.isEnabled = idle
        batchSealCheck.visibility = if (state.batchSealAvailable) View.VISIBLE else View.GONE
        batchSealCheck.isEnabled = idle
        batchSealHelper.visibility = if (state.batchSealAvailable && state.wave4.batchSeal) View.VISIBLE else View.GONE
        batchDnieHint.visibility = if (state.certificateExternal && state.externalBatchAvailable) View.VISIBLE else View.GONE
        updating = true
        batchAction.select(if (state.wave4.batchAction == "cosign") 1 else 0)
        batchSealCheck.isChecked = state.wave4.batchSeal
        updating = false
    }

    private fun renderToggle(view: SectionExpedienteBinding) = with(view) {
        expedienteGroup.visibility = if (expanded) View.VISIBLE else View.GONE
        toggleExpedienteButton.setIconResource(if (expanded) R.drawable.ic_expand_less else R.drawable.ic_expand_more)
        ViewCompat.setStateDescription(toggleExpedienteButton,
            activity.getString(if (expanded) R.string.state_expanded else R.string.state_collapsed))
    }

    private fun showOpeningDatePicker() {
        val manager = activity.supportFragmentManager
        if (manager.findFragmentByTag(EXPEDIENTE_DATE_PICKER) != null) return
        val picker = MaterialDatePicker.Builder.datePicker()
            .setTitleText(R.string.expediente_opening_title)
            .setSelection(viewModel.state.value.wave4.eniFileOpeningDate ?: MaterialDatePicker.todayInUtcMilliseconds())
            .setCalendarConstraints(CalendarConstraints.Builder().setValidator(DateValidatorPointBackward.now()).build())
            .build()
        picker.addOnPositiveButtonClickListener { viewModel.updateEniFileOpeningDate(it) }
        picker.show(manager, EXPEDIENTE_DATE_PICKER)
    }

    /** Código oficial seguido de su descripción del catálogo del motor. */
    private fun codeLabel(code: String): String {
        val key = "eni.codigo.$code"
        val description = EngineText.resolve(activity, key)
        return if (description == key) code else activity.getString(R.string.eni_code_label, code, description)
    }
}
