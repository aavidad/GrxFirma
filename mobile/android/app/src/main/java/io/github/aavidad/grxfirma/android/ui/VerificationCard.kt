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
enum class Verdict { VALID, INTACT_UNCHECKED, INCOMPLETE, REVOCATION_INCONCLUSIVE, INVALID }

/**
 * Tarjeta de resultado de una verificación: un veredicto que no contradice
 * nada de lo que hay debajo, un resumen en lenguaje llano y las evidencias
 * técnicas aparte, plegadas.
 */
object VerificationCard {
    /** Motivo cerrado del motor cuando no puede confirmar la revocación. */
    private const val REVOCATION_INCONCLUSIVE_REASON = "revocación no concluyente"

    fun verdict(v: UiText.Verification): Verdict = when {
        !v.valid && v.reason == REVOCATION_INCONCLUSIVE_REASON -> Verdict.REVOCATION_INCONCLUSIVE
        !v.valid || v.integrityStatus == "invalid" || v.certificateStatus == "invalid" ||
            v.trustStatus == "invalid" -> Verdict.INVALID
        v.reason == REVOCATION_INCONCLUSIVE_REASON -> Verdict.REVOCATION_INCONCLUSIVE
        v.accredited() -> Verdict.VALID
        v.integrityStatus == "valid" -> Verdict.INTACT_UNCHECKED
        else -> Verdict.INCOMPLETE
    }

    @StringRes
    fun title(verdict: Verdict): Int = when (verdict) {
        Verdict.VALID -> R.string.verdict_valid
        Verdict.INTACT_UNCHECKED -> R.string.verdict_intact_unchecked
        Verdict.INCOMPLETE -> R.string.verdict_incomplete
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
        add(context.getString(R.string.verification_coverage, context.getString(when (v.coverage) {
            "full", "total", "whole_document" -> R.string.coverage_full
            "partial", "partial_document" -> R.string.coverage_partial
            "detached" -> R.string.coverage_detached
            else -> R.string.verification_status_unknown
        })))
        val verdict = verdict(v)
        if (showsReason(verdict, v.reason)) add(context.getString(R.string.verification_reason, EngineText.resolve(context, v.reason)))
        if (v.signerSummaries.isNotEmpty()) {
            v.signerSummaries.forEach { signer ->
                add(context.getString(R.string.verification_signer_detail,
                    signer.subject.ifBlank { signer.id }, signer.issuer, signer.fingerprint))
            }
        } else v.signers.forEach { add(context.getString(R.string.verification_signer, it)) }
        v.details.forEach { add(context.getString(R.string.verification_evidence, EngineText.resolve(context, it))) }
        v.warnings.forEach { add(context.getString(R.string.verification_warning, EngineText.resolve(context, it))) }
        v.errors.forEach { add(context.getString(R.string.verification_error, EngineText.resolve(context, it))) }
        add(context.resources.getQuantityString(R.plurals.verification_signers, v.signerCount, v.signerCount))
    }.joinToString("\n")

    /** El motivo del motor («firma PAdES válida») contradiría un veredicto ámbar: solo acompaña a un rechazo. */
    fun showsReason(verdict: Verdict, reason: String): Boolean =
        reason.isNotBlank() && (verdict == Verdict.INVALID || verdict == Verdict.REVOCATION_INCONCLUSIVE)

    /** «CN=Ana Pérez,O=Diputación,C=ES» → «Ana Pérez». Si no hay CN, el nombre completo. */
    fun commonName(distinguishedName: String): String {
        val parts = distinguishedName.split(Regex("(?<!\\\\)[,+]")).map { it.trim() }
        val cn = parts.firstOrNull { it.startsWith("CN=", ignoreCase = true) }?.substring(3)?.replace("\\,", ",")?.trim()
        return cn?.takeIf { it.isNotEmpty() } ?: distinguishedName
    }

    private fun statusLabel(context: Context, status: String): String = context.getString(when (status) {
        "valid" -> R.string.verification_status_valid
        "invalid" -> R.string.verification_status_invalid
        "warning" -> R.string.verification_status_warning
        else -> R.string.verification_status_unknown
    })
}
