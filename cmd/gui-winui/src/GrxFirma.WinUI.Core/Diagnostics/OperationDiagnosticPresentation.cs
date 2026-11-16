// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Diagnostics;

public enum DiagnosticOwner
{
    Computer,
    Browser,
    Portal,
    GovernmentAFirma,
    Unknown,
}

public sealed record OperationDiagnosticPresentation
{
    private const int MaxVisibleSteps = 32;

    public required string Cause { get; init; }
    public required DiagnosticOwner Owner { get; init; }
    public required string OwnerLabel { get; init; }
    public required string Responsibility { get; init; }
    public required string SuggestedAction { get; init; }
    public required string ExpertDetail { get; init; }
    public string? FailureCode { get; init; }
    public required IReadOnlyList<DiagnosticStepPresentation> Steps { get; init; }

    public static OperationDiagnosticPresentation From(
        OperationDiagnostic diagnostic) => From(diagnostic, null);

    // Los textos de respaldo salen del catálogo por clave; lo que manda el
    // motor sigue pasando por TranslateVisibleText. catalogText solo lo
    // cambian las pruebas.
    public static OperationDiagnosticPresentation From(
        OperationDiagnostic diagnostic,
        Func<string, string>? catalogText)
    {
        ArgumentNullException.ThrowIfNull(diagnostic);
        var text = catalogText ?? CatalogLocalizer.Shared.Text;

        var owner = MapOwner(diagnostic.LikelyOwner);
        return new OperationDiagnosticPresentation
        {
            Cause = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.UserMessage,
                512,
                text("winui.diagnostico.presentacion.causa_desconocida"))),
            Owner = owner,
            OwnerLabel = OwnerText(owner, text),
            Responsibility = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.ResponsibilityMessage,
                512,
                text("winui.diagnostico.presentacion.responsable_desconocido"))),
            SuggestedAction = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.SuggestedAction,
                512,
                text("winui.diagnostico.presentacion.accion_desconocida"))),
            ExpertDetail = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.ExpertMessage,
                4096,
                text("winui.diagnostico.respaldo.detalle"))),
            FailureCode = NormalizeFailureCode(diagnostic.FailureCode),
            Steps = diagnostic.Steps
                .Take(MaxVisibleSteps)
                .Select(step => DiagnosticStepPresentation.From(step, text))
                .ToArray(),
        };
    }

    public static DiagnosticOwner MapOwner(string? value) =>
        value?.Trim().ToLowerInvariant() switch
        {
            "app_local" or
            "certificate_store" or
            "certificate_or_device" or
            "environment" or
            "network_proxy" or
            "computer" => DiagnosticOwner.Computer,
            "browser" or
            "browser_integration" or
            "local_web_service" or
            "navegador" => DiagnosticOwner.Browser,
            "remote_service" or
            "portal" or
            "website" => DiagnosticOwner.Portal,
            "government_afirma" or
            "afirma" or
            "@firma" => DiagnosticOwner.GovernmentAFirma,
            _ => DiagnosticOwner.Unknown,
        };

    public static string OwnerText(DiagnosticOwner owner) => OwnerText(owner, null);

    public static string OwnerText(DiagnosticOwner owner, Func<string, string>? catalogText) =>
        (catalogText ?? CatalogLocalizer.Shared.Text)(owner switch
    {
        DiagnosticOwner.Computer => "winui.diagnostico.presentacion.origen_equipo",
        DiagnosticOwner.Browser => "winui.diagnostico.presentacion.origen_navegador",
        DiagnosticOwner.Portal => "winui.diagnostico.presentacion.origen_portal",
        DiagnosticOwner.GovernmentAFirma => "winui.diagnostico.presentacion.origen_afirma",
        _ => "winui.diagnostico.presentacion.origen_desconocido",
    });

    private static string? NormalizeFailureCode(string? value)
    {
        var normalized = value?.Trim().ToUpperInvariant();
        if (string.IsNullOrEmpty(normalized) ||
            normalized.Length > 64 ||
            normalized.Any(character =>
                character is not ('_' or '-' or >= 'A' and <= 'Z' or >= '0' and <= '9')))
        {
            return null;
        }

        return normalized;
    }
}

public sealed record DiagnosticStepPresentation
{
    public required string Code { get; init; }
    public required string Label { get; init; }
    public required DiagnosticStepStatus Status { get; init; }
    public required string StatusIcon { get; init; }
    public required string StatusText { get; init; }
    public required string StatusDisplayText { get; init; }
    public required string Detail { get; init; }
    public required DiagnosticOwner Owner { get; init; }
    public required string OwnerLabel { get; init; }
    public required string OwnerDisplayText { get; init; }
    public required string SuggestedAction { get; init; }
    public required string SuggestedActionDisplayText { get; init; }
    public required string AutomationSummary { get; init; }

    public static DiagnosticStepPresentation From(OperationDiagnosticStep step) => From(step, null);

    public static DiagnosticStepPresentation From(
        OperationDiagnosticStep step,
        Func<string, string>? catalogText)
    {
        ArgumentNullException.ThrowIfNull(step);
        var text = catalogText ?? CatalogLocalizer.Shared.Text;
        var label = CatalogLocalizer.Shared.TranslateVisibleText(
            SafeIpcText.Clean(step.Label, 160, text("winui.diagnostico.paso_observado")));
        var statusText = StatusDescription(step.Status, text);
        var detail = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
            step.UserMessage,
            512,
            step.Status == DiagnosticStepStatus.Unknown
                ? text("winui.diagnostico.presentacion.paso_sin_comprobar")
                : text("winui.diagnostico.presentacion.paso_sin_detalle")));
        var owner = OperationDiagnosticPresentation.MapOwner(step.Owner);
        var ownerLabel = OperationDiagnosticPresentation.OwnerText(owner, text);
        var suggestedAction = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
            step.SuggestedAction,
            512,
            text("winui.diagnostico.presentacion.paso_sin_accion")));

        return new DiagnosticStepPresentation
        {
            Code = SafeIpcText.Clean(step.Code, 64, "unknown"),
            Label = label,
            Status = step.Status,
            StatusIcon = StatusIconText(step.Status),
            StatusText = statusText,
            StatusDisplayText = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                text("winui.diagnostico.presentacion.paso_estado"), statusText),
            Detail = detail,
            Owner = owner,
            OwnerLabel = ownerLabel,
            OwnerDisplayText = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                text("winui.diagnostico.presentacion.paso_responsable"), ownerLabel),
            SuggestedAction = suggestedAction,
            SuggestedActionDisplayText = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                text("winui.diagnostico.presentacion.paso_que_hacer"), suggestedAction),
            AutomationSummary = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                text("winui.diagnostico.presentacion.paso_resumen"),
                label, statusText, ownerLabel, detail, suggestedAction),
        };
    }

    public static string StatusIconText(DiagnosticStepStatus status) => status switch
    {
        DiagnosticStepStatus.Success => "✓",
        DiagnosticStepStatus.Failure => "✕",
        DiagnosticStepStatus.Skipped => "—",
        _ => "?",
    };

    public static string StatusDescription(DiagnosticStepStatus status) => StatusDescription(status, null);

    public static string StatusDescription(DiagnosticStepStatus status, Func<string, string>? catalogText) =>
        (catalogText ?? CatalogLocalizer.Shared.Text)(status switch
    {
        DiagnosticStepStatus.Success => "winui.diagnostico.presentacion.estado_correcto",
        DiagnosticStepStatus.Failure => "winui.diagnostico.presentacion.estado_fallo",
        DiagnosticStepStatus.Skipped => "winui.diagnostico.presentacion.estado_omitido",
        _ => "winui.diagnostico.presentacion.estado_no_comprobado",
    });
}
