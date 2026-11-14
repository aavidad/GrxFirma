// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Diagnostics;

public sealed record LocalFailureEvidence(
    CheckCertificatesSummaryResult? Certificates = null,
    ClockDiagnosticResult? Clock = null);

public sealed record FailureEvidenceRow(string Label, string Detail);

// Separa el fallo original de las lecturas posteriores. Un inventario correcto
// ahora no demuestra que el certificado elegido estuviera disponible al fallar.
public static class ContextualFailureAssistant
{
    private static string Text(string key) => CatalogLocalizer.Shared.Text(key);

    public static string ActionKey(OperationDiagnostic diagnostic) =>
        diagnostic.FailureCode?.ToUpperInvariant() switch
        {
            "SAF_08" => "winui.diagnostico.abra_certificados_y_revise_el_almacen_la",
            "SAF_44" => "winui.l27.assistant.save",
            "CONNECTION_CLOSED" or "CONNECTION_FAILED" =>
                "winui.diagnostico.cierre_la_aplicacion_abrala_desde_el",
            "SAF_26" or "SAF_27" => "winui.l27.assistant.connection",
            _ => diagnostic.Category == "certificate_store"
                ? "winui.diagnostico.abra_certificados_y_revise_el_almacen_la"
                : "winui.l27.assistant.next",
        };

    public static string OperationKey(string? action) => action switch
    {
        "sign" or "sign_batch" or "sign_multicosign" or "protect_sign" => "winui.comun.firmar",
        "verify" => "winui.comun.verificar",
        "protect" or "unprotect" => "winui.comun.proteger",
        "certificates" or "check_certificates" or "import_certificate" => "winui.parity.support.goal.certificate",
        _ => "report.status.unknown",
    };

    public static OperationDiagnostic ForDisplay(OperationDiagnostic diagnostic)
    {
        var safe = OperationDiagnosticMapper.Sanitize(diagnostic);
        if (safe.FailureCode == "USER_CANCELLED" ||
            (safe.FailureCode is null && !safe.Steps.Any(step => step.Status == DiagnosticStepStatus.Failure)))
            return safe;
        return safe with
        {
            // Un código de comunicación o una atribución heredada no prueban
            // que el fallo sea responsabilidad del equipo, la sede o @firma.
            LikelyOwner = "unknown",
            UserMessage = safe.FailureCode is "SAF_26" or "SAF_27"
                ? Text("winui.l27.assistant.connection")
                : safe.Category is "remote_service" or "government_afirma" or "network_proxy"
                    ? Text("winui.diagnostico.se_ha_detectado_un_fallo_pero_su_origen")
                    : safe.UserMessage,
            ResponsibilityMessage = Text("winui.diagnostico.la_evidencia_disponible_no_permite"),
            SuggestedAction = Text(ActionKey(safe)),
            Steps = safe.Steps.Select(step => step.EvidenceRef == "phase:operation"
                ? step with { Owner = "unknown", SuggestedAction = Text(ActionKey(safe)) } : step).ToArray(),
        };
    }

    public static IReadOnlyList<FailureEvidenceRow> Rows(
        OperationDiagnostic diagnostic, LocalFailureEvidence? local = null)
    {
        var safe = OperationDiagnosticMapper.Sanitize(diagnostic);
        var unknown = Text("report.status.unknown");
        string Observed(string code)
        {
            // La atribución probable o la categoría del fallo general no son
            // evidencia de haber comprobado por separado una red o un servidor.
            var step = safe.Steps.LastOrDefault(step =>
                step.Code.Equals(code, StringComparison.OrdinalIgnoreCase) &&
                step.EvidenceRef is not null &&
                step.Status is DiagnosticStepStatus.Success or DiagnosticStepStatus.Failure);
            return step is null ? unknown : DiagnosticStepPresentation.From(step).AutomationSummary;
        }
        var certificates = local?.Certificates is { HasCoherentCounts: true } counts
            ? string.Format(System.Globalization.CultureInfo.CurrentCulture,
                Text("winui.l27.assistant.certificates"), counts.OkCount, counts.FailCount)
            : Observed("certificate_inventory");
        // La fase local describe lectura y servicio de hora; no sincronización.
        var clock = local?.Clock is { IsCoherent: true } reading
            ? reading.Steps.Single(step => step.Code == "local_clock").ToOperationStep()
            : null;
        if (clock is not null)
            clock = OperationDiagnosticMapper.Sanitize(new() { Steps = [clock] }).Steps.Single();
        return
        [
            new(Text("winui.diagnostico.certificados_utilizables"), certificates),
            new(Text("winui.diagnostico.reloj_local"), clock is null
                ? Observed("local_clock") : DiagnosticStepPresentation.From(clock).AutomationSummary),
            new(Text("winui.diagnostico.hora_del_servidor_remoto_observado"), Observed("remote_clock")),
            new(Text("winui.l27.assistant.network"), Observed("network_proxy")),
            new(Text("winui.l27.assistant.portal"), Observed("remote_service")),
            new(Text("winui.diagnostico.plataforma_firma"), Observed("government_afirma")),
        ];
    }

    public static async Task<LocalFailureEvidence> CheckLocalAsync(
        DesktopOperationSession session, CancellationToken cancellationToken)
    {
        cancellationToken.ThrowIfCancellationRequested();
        CheckCertificatesSummaryResult? certificates = null;
        ClockDiagnosticResult? clock = null;
        // Solo contratos existentes, sin direcciones ni datos de la operación.
        // Cada consulta es independiente: un fallo deja ese dato sin comprobar.
        if (session.TryGetOperations(DesktopOperationActions.CheckCertificates, out var operations))
        {
            try
            {
                var result = await operations.CheckCertificatesAsync(cancellationToken);
                if (result.IsSuccess && result.Outcome == "success" &&
                    result.Data is { HasCoherentCounts: true }) certificates = result.Data;
            }
            catch (Exception) when (!cancellationToken.IsCancellationRequested) { }
        }
        cancellationToken.ThrowIfCancellationRequested();
        if (session.TryGetOperations(DesktopOperationActions.ClockDiagnostics, out operations))
        {
            try
            {
                var result = await operations.GetClockDiagnosticsAsync(cancellationToken);
                if (result.IsSuccess && result.Outcome == "success" &&
                    result.Data is { IsCoherent: true }) clock = result.Data;
            }
            catch (Exception) when (!cancellationToken.IsCancellationRequested) { }
        }
        cancellationToken.ThrowIfCancellationRequested();
        return new(certificates, clock);
    }
}
