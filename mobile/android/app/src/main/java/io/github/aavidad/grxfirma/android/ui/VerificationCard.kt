// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.content.Context
import androidx.annotation.ColorRes
import androidx.annotation.StringRes
import io.github.aavidad.grxfirma.android.R

/** Veredicto de una verificación, de mejor a peor. Decide título y color de la tarjeta. */
enum class Verdict { VALID, INTACT_UNCHECKED, INTACT_NOT_CONFIRMED, INCOMPLETE, INTEGRITY_WARNING, REVOCATION_INCONCLUSIVE, INVALID }

/**
 * Tarjeta de resultado de una verificación: un veredicto que no contradice
 * nada de lo que hay debajo, un resumen en lenguaje llano y las evidencias
 * técnicas aparte, plegadas.
 */
object VerificationCard {
    /** Motivo cerrado del motor cuando no puede confirmar la revocación. */
    private const val REVOCATION_INCONCLUSIVE_REASON = "revocación no concluyente"

    /**
     * Rojo solo cuando algo ha fallado de verdad: integridad, certificado o
     * confianza inválidos, o una firma rechazada sin integridad comprobada.
     * Con la integridad válida y nada inválido, una firma que el motor no da
     * por válida (p. ej. la confianza quedó sin evaluar porque falta activar
     * una opción) es ámbar, como en el escritorio. Con la integridad «warning»
     * (p. ej. XML firmado con la compatibilidad de canonicalización histórica)
     * la firma no es válida pero nada ha fallado: también ámbar, como WinUI y Qt.
     */
    fun verdict(v: UiText.Verification): Verdict = when {
        !v.valid && v.reason == REVOCATION_INCONCLUSIVE_REASON -> Verdict.REVOCATION_INCONCLUSIVE
        v.integrityStatus == "invalid" || v.certificateStatus == "invalid" || v.trustStatus == "invalid" -> Verdict.INVALID
        !v.valid && v.integrityStatus == "valid" -> Verdict.INTACT_NOT_CONFIRMED
        !v.valid && v.integrityStatus == "warning" -> Verdict.INTEGRITY_WARNING
        !v.valid -> Verdict.INVALID
        v.reason == REVOCATION_INCONCLUSIVE_REASON -> Verdict.REVOCATION_INCONCLUSIVE
        v.accredited() -> Verdict.VALID
        v.integrityStatus == "valid" -> Verdict.INTACT_UNCHECKED
        else -> Verdict.INCOMPLETE
    }

    @StringRes
    fun title(verdict: Verdict): Int = when (verdict) {
        Verdict.VALID -> R.string.verdict_valid
        Verdict.INTACT_UNCHECKED -> R.string.verdict_intact_unchecked
        Verdict.INTACT_NOT_CONFIRMED -> R.string.verdict_intact_not_confirmed
        Verdict.INCOMPLETE -> R.string.verdict_incomplete
        Verdict.INTEGRITY_WARNING -> R.string.verdict_integrity_warning
        Verdict.REVOCATION_INCONCLUSIVE -> R.string.verdict_revocation_inconclusive
        Verdict.INVALID -> R.string.verdict_invalid
    }

    @ColorRes
    fun color(verdict: Verdict): Int = when (verdict) {
        Verdict.VALID -> R.color.primary
        Verdict.INVALID -> R.color.error
        else -> R.color.status_warning
    }

    /** Lo que una persona necesita saber, sin jerga: quién firmó, si se ha tocado y si el certificado vale. */
    fun summary(v: UiText.Verification): List<UiText> = buildList {
        val names = v.signerSummaries.map { it.subject.ifBlank { it.id } }.ifEmpty { v.signers }
        names.filter { it.isNotBlank() }.forEach {
            add(UiText.Resource(R.string.verification_signed_by, listOf(commonName(it))))
        }
        add(UiText.Resource(when (v.integrityStatus) {
            "valid" -> R.string.verification_integrity_ok
            "invalid" -> R.string.verification_integrity_bad
            else -> R.string.verification_integrity_unknown
        }))
        if (v.coverage == "partial" || v.coverage == "partial_document") add(UiText.Resource(R.string.verification_partial))
        add(UiText.Resource(when {
            v.certificateStatus == "invalid" || v.trustStatus == "invalid" -> R.string.verification_certificate_bad
            v.certificateStatus == "valid" && v.trustStatus == "valid" -> R.string.verification_certificate_ok
            else -> R.string.verification_certificate_unknown
        }))
        if (verdict(v) == Verdict.REVOCATION_INCONCLUSIVE) add(UiText.Resource(R.string.verification_revocation_unknown))
        // Estos avisos deben verse sin desplegar las evidencias técnicas.
        addAll(exceptionWarnings(v))
    }

    /** Avisos de una excepción insegura activada: van en el resumen y destacados. */
    fun exceptionWarnings(v: UiText.Verification): List<UiText> =
        v.warnings.distinct().filter { it.startsWith("excepciones.") && EngineKeys.isClosed(it) }
            .map { UiText.Resource(R.string.verification_warning, listOf(UiText.Engine(it))) }

    /** Dónde está cada aviso ya resuelto dentro del texto de la tarjeta, a partir de [from]. */
    fun highlightRanges(text: CharSequence, warnings: List<String>, from: Int = 0): List<IntRange> =
        warnings.filter { it.isNotEmpty() }.mapNotNull { warning ->
            text.indexOf(warning, from).takeIf { it >= 0 }?.let { it until it + warning.length }
        }

    /** Evidencias del motor para quien las necesite; el motivo solo se cita si explica un rechazo. */
    fun technical(context: Context, v: UiText.Verification): String = buildList {
        add(context.getString(R.string.verification_integrity, statusLabel(context, v.integrityStatus)))
        add(context.getString(R.string.verification_certificate, statusLabel(context, v.certificateStatus)))
        add(context.getString(R.string.verification_trust, statusLabel(context, v.trustStatus)))
        add(context.getString(R.string.verification_revocation, context.getString(when (v.revocationMode) {
            "embedded_evidence_only" -> R.string.revocation_embedded_only
            "online" -> R.string.revocation_online
            else -> R.string.revocation_not_available
        })))
        if (v.format.isNotBlank()) add(context.getString(R.string.verification_format, v.format))
        add(context.getString(R.string.verification_coverage, context.getString(coverageValue(v))))
        val verdict = verdict(v)
        val reasonShown = showsReason(verdict, v.reason)
        if (reasonShown) add(context.getString(R.string.verification_reason, EngineText.resolve(context, v.reason)))
        if (v.signerSummaries.isNotEmpty()) {
            v.signerSummaries.forEach { signer ->
                add(context.getString(R.string.verification_signer_detail,
                    signer.subject.ifBlank { signer.id }, signer.issuer, signer.fingerprint))
            }
        } else v.signers.forEach { add(context.getString(R.string.verification_signer, it)) }
        v.details.forEach { add(context.getString(R.string.verification_evidence, EngineText.resolve(context, it))) }
        val (warnings, errors) = withoutRepeats(v, reasonShown)
        warnings.forEach { add(context.getString(R.string.verification_warning, EngineText.resolve(context, it))) }
        errors.forEach { add(context.getString(R.string.verification_error, EngineText.resolve(context, it))) }
        add(context.resources.getQuantityString(R.plurals.verification_signers, v.signerCount, v.signerCount))
    }.joinToString("\n")

    /**
     * Cada texto sale una vez en las evidencias: un aviso o un error que repite
     * el motivo ya mostrado (o un aviso que también es error) no se vuelve a pintar.
     */
    fun withoutRepeats(v: UiText.Verification, reasonShown: Boolean): Pair<List<String>, List<String>> {
        val seen = if (reasonShown) mutableSetOf(v.reason) else mutableSetOf()
        val errors = v.errors.distinct().filter { seen.add(it) }
        val warnings = v.warnings.distinct().filter { it !in seen }
        return warnings to errors
    }

    /**
     * El motivo del motor («firma PAdES válida») contradiría un veredicto ámbar:
     * solo acompaña a un rechazo o explica por qué una firma íntegra no se confirma.
     */
    fun showsReason(verdict: Verdict, reason: String): Boolean =
        reason.isNotBlank() && (verdict == Verdict.INVALID || verdict == Verdict.REVOCATION_INCONCLUSIVE ||
            verdict == Verdict.INTACT_NOT_CONFIRMED || verdict == Verdict.INTEGRITY_WARNING)

    /** «CN=Ana Pérez,O=Diputación,C=ES» → «Ana Pérez». Si no hay CN, el nombre completo. */
    fun commonName(distinguishedName: String): String {
        val parts = distinguishedName.split(Regex("(?<!\\\\)[,+]")).map { it.trim() }
        val cn = parts.firstOrNull { it.startsWith("CN=", ignoreCase = true) }?.substring(3)?.replace("\\,", ",")?.trim()
        return cn?.takeIf { it.isNotEmpty() } ?: distinguishedName
    }

    /**
     * Resultado en palabras de cada comprobación que la tarjeta muestra junto a su «?»
     * («Integridad: válida»): sin él solo se veía la etiqueta y el lector de pantalla
     * no decía el valor.
     */
    data class Aspect(@StringRes val label: Int, @StringRes val value: Int)

    fun aspects(v: UiText.Verification): List<Aspect> = listOf(
        Aspect(R.string.ayuda_tema_integridad, statusValue(v.integrityStatus)),
        Aspect(R.string.ayuda_tema_confianza, statusValue(v.trustStatus)),
        Aspect(R.string.ayuda_tema_cobertura, coverageValue(v)),
    )

    @StringRes
    fun statusValue(status: String): Int = when (status) {
        "valid" -> R.string.verification_status_valid
        "invalid" -> R.string.verification_status_invalid
        "warning" -> R.string.verification_status_warning
        else -> R.string.verification_status_unknown
    }

    @StringRes
    fun coverageValue(v: UiText.Verification): Int = when (v.coverage) {
        "full", "total", "whole_document" -> R.string.coverage_full
        "partial", "partial_document" -> R.string.coverage_partial
        "detached" -> R.string.coverage_detached
        else -> R.string.verification_status_unknown
    }

    private fun statusLabel(context: Context, status: String): String = context.getString(statusValue(status))
}
