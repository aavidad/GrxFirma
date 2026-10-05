// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import androidx.annotation.StringRes
import android.content.Context
import es.dipgra.grxfirma.android.R
import es.dipgra.grxfirma.android.model.CertificateDetail
import es.dipgra.grxfirma.android.model.EngineDiagnostics
import es.dipgra.grxfirma.android.model.RevocationCheck
import es.dipgra.grxfirma.android.model.TsaProbe
import es.dipgra.grxfirma.android.model.UpdateCheck
import es.dipgra.grxfirma.android.model.VeriFactuQr
import es.dipgra.grxfirma.android.model.VeriFactuReport
import org.json.JSONObject
import java.net.URI
import kotlin.math.abs

/** Textos del panel del certificado: caducidad, tipo, NIF y clave. */
object CertificateText {
    @StringRes
    fun kindLabel(kind: String): Int = when (kind) {
        "fisica" -> R.string.cert_kind_fisica
        "representacion" -> R.string.cert_kind_representacion
        "sello" -> R.string.cert_kind_sello
        "empleado_publico" -> R.string.cert_kind_empleado_publico
        else -> R.string.cert_kind_desconocido
    }

    /** Aviso de caducidad con palabra, no solo color. */
    fun expiry(detail: CertificateDetail): UiText = when (detail.status) {
        "expired" -> UiText.Resource(R.string.cert_expired, listOf(UiText.DateTime(detail.notAfter)))
        "not_yet_valid" -> UiText.Resource(R.string.cert_not_yet_valid, listOf(UiText.DateTime(detail.notBefore)))
        "expiring_soon" -> if (detail.daysLeft <= 0)
            UiText.Resource(R.string.cert_expires_today, listOf(UiText.DateTime(detail.notAfter)))
        else UiText.Resource(R.string.cert_expires_soon, listOf(UiText.Plural(R.plurals.cert_days_left, detail.daysLeft),
            UiText.DateTime(detail.notAfter)))
        else -> UiText.Resource(R.string.cert_valid_until, listOf(UiText.DateTime(detail.notAfter)))
    }

    fun warns(detail: CertificateDetail): Boolean = detail.status != "valid"

    fun lines(detail: CertificateDetail): UiText.Lines = UiText.Lines(buildList {
        add(expiry(detail))
        add(UiText.Resource(R.string.cert_kind, listOf(UiText.Resource(kindLabel(detail.kind)))))
        if (detail.nif.isNotBlank()) add(UiText.Resource(R.string.cert_nif, listOf(detail.nif)))
        if (detail.organization.isNotBlank()) add(UiText.Resource(R.string.cert_organization, listOf(detail.organization)))
        if (detail.keyType.isNotBlank()) add(UiText.Resource(R.string.cert_key, listOf(detail.keyType, detail.keyBits.toString())))
        add(UiText.Resource(if (detail.external) R.string.cert_origin_dnie else R.string.cert_origin_file))
        if (detail.fingerprint.isNotBlank()) add(UiText.Resource(R.string.cert_fingerprint, listOf(detail.fingerprint)))
    })

    /**
     * Detalle listo para pintar: solo la línea de caducidad va en color de
     * aviso (y en negrita) cuando avisa; el resto, en el color normal.
     */
    fun styled(context: Context, detail: CertificateDetail, heading: String? = null): CharSequence {
        val text = android.text.SpannableStringBuilder()
        if (heading != null) text.append(heading).append("\n")
        val all = lines(detail).lines
        val start = text.length
        text.append(all.first().resolve(context))
        if (warns(detail)) {
            text.setSpan(android.text.style.ForegroundColorSpan(androidx.core.content.ContextCompat.getColor(context, R.color.status_warning)),
                start, text.length, android.text.Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            text.setSpan(android.text.style.StyleSpan(android.graphics.Typeface.BOLD), start, text.length,
                android.text.Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }
        all.drop(1).forEach { text.append("\n").append(it.resolve(context)) }
        return text
    }
}

/** Resultado de la consulta OCSP/CRL en lenguaje llano. */
object RevocationText {
    fun lines(check: RevocationCheck): UiText.Lines = UiText.Lines(buildList {
        val noService = !check.hasOcsp && !check.hasCrl
        add(UiText.Resource(when {
            check.status == "valid" -> R.string.revocation_valid
            check.status == "revoked" -> R.string.revocation_revoked
            noService -> R.string.revocation_no_service
            check.status == "inconclusive" -> R.string.revocation_inconclusive
            else -> R.string.revocation_unavailable
        }))
        if (check.revokedAt.isNotBlank()) add(UiText.Resource(R.string.revocation_revoked_since, listOf(UiText.DateTime(check.revokedAt, true))))
        if (check.checkedAt.isNotBlank()) add(UiText.Resource(R.string.revocation_checked, listOf(UiText.DateTime(check.checkedAt, true))))
        add(UiText.Resource(R.string.revocation_scope))
    })
}

/** Datos que la app conoce de sí misma, sin nada de la persona usuaria. */
data class AppFacts(
    val appVersion: String,
    val sourceCommit: String,
    val coreSha256: String,
    val androidRelease: String,
    val sdk: Int,
    val language: String,
    val deviceTimeIso: String,
    val timeZone: String,
)

/**
 * Informe de diagnóstico. Solo incluye versiones, huella del núcleo, reloj,
 * idioma y el host de la TSA (nunca la ruta ni parámetros), sin titular,
 * NIF, huella del certificado ni nombres de documentos.
 */
object DiagnosticsText {
    const val MAX_SKEW_SECONDS = 120L

    fun lines(facts: AppFacts, engine: EngineDiagnostics?, tsaUrl: String, probe: TsaProbe?): UiText.Lines = UiText.Lines(buildList {
        add(UiText.Resource(R.string.diag_app, listOf(facts.appVersion, facts.sourceCommit.take(12).ifBlank { "-" })))
        if (engine != null) {
            add(UiText.Resource(R.string.diag_engine, listOf(engine.engineVersion.ifBlank { "-" }, engine.contractVersion)))
            add(UiText.Resource(R.string.diag_runtime, listOf(engine.goVersion.ifBlank { "-" }, engine.architecture.ifBlank { "-" })))
        } else add(UiText.Resource(R.string.diag_engine_unavailable))
        add(UiText.Resource(R.string.diag_core_sha, listOf(facts.coreSha256.ifBlank { "-" })))
        add(UiText.Resource(R.string.diag_android, listOf(facts.androidRelease, facts.sdk)))
        add(UiText.Resource(R.string.diag_language, listOf(facts.language.ifBlank { "-" })))
        add(UiText.Resource(R.string.diag_clock, listOf(UiText.DateTime(facts.deviceTimeIso, true), facts.timeZone)))
        if (engine != null && engine.engineTimeUtc.isNotBlank()) {
            add(UiText.Resource(R.string.diag_engine_clock, listOf(engine.engineTimeUtc)))
        }
        val host = tsaHost(tsaUrl)
        add(if (host.isEmpty()) UiText.Resource(R.string.diag_tsa_none) else UiText.Resource(R.string.diag_tsa_host, listOf(host)))
        if (probe != null) addAll(tsaLines(probe))
    })

    fun tsaLines(probe: TsaProbe): List<UiText> = buildList {
        add(UiText.Resource(when (probe.status) {
            "ok" -> R.string.diag_tsa_ok
            "invalid_url" -> R.string.diag_tsa_invalid_url
            "timeout" -> R.string.diag_tsa_timeout
            "rejected" -> R.string.diag_tsa_rejected
            "unreachable" -> R.string.diag_tsa_unreachable
            else -> R.string.diag_tsa_bad_response
        }, if (probe.status == "ok") listOf(probe.elapsedMillis) else emptyList()))
        if (probe.status == "ok") {
            val skew = probe.skewSeconds
            add(when {
                abs(skew) <= MAX_SKEW_SECONDS -> UiText.Resource(R.string.diag_clock_ok, listOf(UiText.Plural(R.plurals.seconds, abs(skew).toInt())))
                skew > 0 -> UiText.Resource(R.string.diag_clock_ahead, listOf(UiText.Plural(R.plurals.seconds, skew.coerceAtMost(Int.MAX_VALUE.toLong()).toInt())))
                else -> UiText.Resource(R.string.diag_clock_behind, listOf(UiText.Plural(R.plurals.seconds, (-skew).coerceAtMost(Int.MAX_VALUE.toLong()).toInt())))
            })
        }
        if (!probe.https && probe.status != "invalid_url") add(UiText.Resource(R.string.diag_tsa_http))
    }

    /** Solo el host: la ruta o la consulta de una TSA privada podrían ser sensibles. */
    fun tsaHost(url: String): String = try {
        URI(url.trim()).host.orEmpty().take(253)
    } catch (_: Exception) { "" }
}

/** Mensajes de la comprobación de versión. */
object UpdateText {
    fun lines(check: UpdateCheck): UiText.Lines = UiText.Lines(buildList {
        add(when (check.status) {
            "newer" -> UiText.Resource(R.string.update_newer, listOf(check.latest, check.current))
            "current" -> UiText.Resource(R.string.update_current, listOf(check.current))
            "not_comparable" -> UiText.Resource(R.string.update_not_comparable, listOf(check.latest, check.current))
            "no_releases" -> UiText.Resource(R.string.update_no_releases)
            else -> UiText.Resource(when (check.errorCode) {
                "update_timeout" -> R.string.update_error_timeout
                "update_rate_limited" -> R.string.update_error_rate_limited
                "update_service_unavailable" -> R.string.update_error_service
                "update_network_unavailable", "update_proxy_unavailable" -> R.string.update_error_network
                else -> R.string.update_error_generic
            })
        })
        if (check.status == "newer") add(UiText.Resource(R.string.update_install_hint))
        add(UiText.Resource(R.string.update_privacy))
    })
}

/** Lectura del QR tributario y respuesta de la AEAT. */
object QrText {
    /** [includeTest] = false cuando la pantalla pinta aparte, en color de aviso, el entorno de pruebas. */
    fun lines(qr: VeriFactuQr, locale: java.util.Locale = java.util.Locale.getDefault(), includeTest: Boolean = true): UiText.Lines =
        UiText.Lines(buildList {
            add(UiText.Resource(R.string.issue_line, listOf(UiText.Engine("verifactu.qr_nif"), qr.nif)))
            add(UiText.Resource(R.string.issue_line, listOf(UiText.Engine("verifactu.qr_number"), qr.number)))
            add(UiText.Resource(R.string.issue_line, listOf(UiText.Engine("verifactu.qr_date"), qr.date)))
            add(UiText.Resource(R.string.issue_line, listOf(UiText.Engine("verifactu.qr_amount"), amount(qr.amount, locale))))
            add(UiText.Resource(if (qr.verifiable) R.string.qr_verifactu else R.string.qr_not_verifactu))
            if (includeTest && qr.test) add(UiText.Resource(R.string.qr_test_environment))
        })

    /** «241.4» del QR tributario → «241,40 €» en el idioma del móvil. Si no es un número, tal cual. */
    fun amount(raw: String, locale: java.util.Locale): String = try {
        val value = java.math.BigDecimal(raw.trim())
        java.text.NumberFormat.getCurrencyInstance(locale).apply {
            currency = java.util.Currency.getInstance("EUR")
        }.format(value)
    } catch (_: Exception) { raw }
}

/** JSON exportado: el informe traducido y la respuesta del motor, sin cambios. */
fun VeriFactuText.exportJson(report: VeriFactuReport, localized: String): String {
    val result = try { JSONObject(report.reportJson) } catch (_: Exception) { JSONObject() }
    return JSONObject()
        .put("report", localized)
        .put("valid", report.valid)
        .put("result", result)
        .toString(2)
}
