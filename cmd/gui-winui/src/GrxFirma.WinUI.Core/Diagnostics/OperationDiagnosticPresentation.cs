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
        OperationDiagnostic diagnostic)
    {
        ArgumentNullException.ThrowIfNull(diagnostic);

        var owner = MapOwner(diagnostic.LikelyOwner);
        return new OperationDiagnosticPresentation
        {
            Cause = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.UserMessage,
                512,
                "No se ha podido determinar una causa sencilla.")),
            Owner = owner,
            OwnerLabel = OwnerText(owner),
            Responsibility = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.ResponsibilityMessage,
                512,
                "No se ha podido determinar quién debe revisar el problema.")),
            SuggestedAction = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.SuggestedAction,
                512,
                "No hay una acción concreta registrada. Conserve el código para soporte.")),
            ExpertDetail = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
                diagnostic.ExpertMessage,
                4096,
                "No hay detalle técnico disponible.")),
            FailureCode = NormalizeFailureCode(diagnostic.FailureCode),
            Steps = diagnostic.Steps
                .Take(MaxVisibleSteps)
                .Select(DiagnosticStepPresentation.From)
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

    public static string OwnerText(DiagnosticOwner owner) =>
        CatalogLocalizer.Shared.TranslateVisibleText(owner switch
    {
        DiagnosticOwner.Computer => "Este equipo o la aplicación local",
        DiagnosticOwner.Browser => "El navegador",
        DiagnosticOwner.Portal => "El portal o sede electrónica",
        DiagnosticOwner.GovernmentAFirma => "La plataforma @firma",
        _ => "Responsable desconocido",
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

    public static DiagnosticStepPresentation From(OperationDiagnosticStep step)
    {
        ArgumentNullException.ThrowIfNull(step);
        var label = CatalogLocalizer.Shared.TranslateVisibleText(
            SafeIpcText.Clean(step.Label, 160, "Paso observado"));
        var statusText = StatusDescription(step.Status);
        var detail = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
            step.UserMessage,
            512,
            step.Status == DiagnosticStepStatus.Unknown
                ? "Este paso no se ha comprobado."
                : "Sin detalle adicional."));
        var owner = OperationDiagnosticPresentation.MapOwner(step.Owner);
        var ownerLabel = OperationDiagnosticPresentation.OwnerText(owner);
        var suggestedAction = CatalogLocalizer.Shared.TranslateVisibleText(SafeIpcText.Clean(
            step.SuggestedAction,
            512,
            "No hay una acción específica registrada para este paso."));

        return new DiagnosticStepPresentation
        {
            Code = SafeIpcText.Clean(step.Code, 64, "unknown"),
            Label = label,
            Status = step.Status,
            StatusIcon = StatusIconText(step.Status),
            StatusText = statusText,
            StatusDisplayText = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                CatalogLocalizer.Shared.Text("Estado: {0}"), statusText),
            Detail = detail,
            Owner = owner,
            OwnerLabel = ownerLabel,
            OwnerDisplayText = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                CatalogLocalizer.Shared.Text("Responsable probable: {0}"), ownerLabel),
            SuggestedAction = suggestedAction,
            SuggestedActionDisplayText = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                CatalogLocalizer.Shared.Text("Qué hacer ahora: {0}"), suggestedAction),
            AutomationSummary = string.Format(
                System.Globalization.CultureInfo.CurrentCulture,
                CatalogLocalizer.Shared.Text(
                    "{0}. Estado: {1}. Responsable probable: {2}. {3} Qué hacer ahora: {4}"),
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

    public static string StatusDescription(DiagnosticStepStatus status) =>
        CatalogLocalizer.Shared.TranslateVisibleText(status switch
    {
        DiagnosticStepStatus.Success => "Correcto",
        DiagnosticStepStatus.Failure => "Falló",
        DiagnosticStepStatus.Skipped => "Omitido",
        _ => "No comprobado",
    });
}
