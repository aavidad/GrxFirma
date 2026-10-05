// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.files.DocumentPolicy
import es.dipgra.grxfirma.android.model.EngineIssue
import es.dipgra.grxfirma.android.model.VeriFactuReport
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale

/** Informe Veri*Factu por registro, con las claves del motor ya localizables. */
object VeriFactuText {
    fun lines(report: VeriFactuReport): UiText.Lines = UiText.Lines(buildList {
        add(UiText.Plural(R.plurals.verifactu_record_count, report.records.size))
        if (report.errors > 0) add(UiText.Plural(R.plurals.verifactu_error_count, report.errors))
        if (report.warnings > 0) add(UiText.Plural(R.plurals.verifactu_warning_count, report.warnings))
        add(UiText.Engine("verifactu.scope"))
        for (record in report.records) {
            add(UiText.Resource(R.string.verifactu_record_header, listOf(record.file, record.type.ifBlank { "XML" })))
            if (record.calculatedHash.isNotBlank()) {
                add(UiText.Resource(R.string.issue_line, listOf(UiText.Engine("verifactu.hash_label"), record.calculatedHash)))
            }
            record.issues.forEach { add(issue(it)) }
            if (record.issues.isEmpty()) add(UiText.Engine("verifactu.valid"))
        }
    })

    fun issue(issue: EngineIssue): UiText = UiText.Resource(
        if (issue.level == "warning") R.string.issue_warning_line else R.string.issue_line,
        listOf(issue.field, UiText.Engine(issue.key)),
    )
}

/** Reglas del formulario ENI que no dependen de la pantalla. */
object EniForm {
    const val MAX_ORGANS = 16
    private val DIR3 = Regex("[A-Z][0-9]{8}")
    private val RFC3339 = DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ssXXX", Locale.ROOT)

    /** Códigos DIR3 separados por comas, espacios o punto y coma. */
    fun organs(raw: String): List<String> = raw.split(',', ';', ' ', '\n')
        .map { it.trim().uppercase(Locale.ROOT) }
        .filter { it.isNotEmpty() }
        .distinct()
        .take(MAX_ORGANS + 1)

    fun organsValid(organs: List<String>): Boolean =
        organs.isNotEmpty() && organs.size <= MAX_ORGANS && organs.all { DIR3.matches(it) }

    /**
     * El calendario de Material entrega la medianoche UTC del día elegido. El
     * ENI recibe ese día a las 00:00 en la zona del dispositivo, en RFC 3339.
     */
    fun captureDate(utcMidnight: Long?, zone: ZoneId = ZoneId.systemDefault()): String {
        if (utcMidnight == null) return ""
        val day = Instant.ofEpochMilli(utcMidnight).atZone(ZoneOffset.UTC).toLocalDate()
        return day.atStartOfDay(zone).format(RFC3339)
    }

    fun outputName(signatureName: String): String {
        val base = signatureName.substringBeforeLast('.').ifBlank { "documento" }.take(120)
        return DocumentPolicy.sanitizeDisplayName("$base.eni.xml", "documento.eni.xml")
    }
}
