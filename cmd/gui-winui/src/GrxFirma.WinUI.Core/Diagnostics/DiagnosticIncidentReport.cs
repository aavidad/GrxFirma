// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Diagnostics;

/// <summary>
/// Genera un informe local acotado a partir del contrato visual saneado. El
/// documento no incorpora rutas, payloads, certificados, firmas, secretos,
/// mensajes de excepción ni trazas de pila.
/// </summary>
public static class DiagnosticIncidentReport
{
    public const string Schema = "grxfirma-winui-incident-v1";

    private static readonly JsonSerializerOptions SerializerOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    public static string CreateJson(
        OperationDiagnostic diagnostic,
        DateTimeOffset generatedAtUtc)
    {
        ArgumentNullException.ThrowIfNull(diagnostic);

        var safeDiagnostic = OperationDiagnosticMapper.Sanitize(diagnostic);
        var presentation = OperationDiagnosticPresentation.From(
            safeDiagnostic);
        var correlation = safeDiagnostic.RequestId is null &&
            safeDiagnostic.TraceId is null &&
            safeDiagnostic.Action is null
            ? null
            : new IncidentCorrelation(
                safeDiagnostic.RequestId,
                safeDiagnostic.TraceId,
                safeDiagnostic.Action);
        var report = new IncidentReport(
            Schema,
            generatedAtUtc.ToUniversalTime(),
            correlation,
            new IncidentSummary(
                presentation.Cause,
                OwnerCode(presentation.Owner),
                presentation.OwnerLabel,
                presentation.Responsibility,
                presentation.SuggestedAction,
                presentation.FailureCode,
                presentation.ExpertDetail),
            presentation.Steps
                .Select(step => new IncidentStep(
                    step.Code,
                    step.Label,
                    StatusCode(step.Status),
                    OwnerCode(step.Owner),
                    step.OwnerLabel,
                    step.Detail,
                    step.SuggestedAction))
                .ToArray());

        return JsonSerializer.Serialize(report, SerializerOptions);
    }

    private static string OwnerCode(DiagnosticOwner owner) => owner switch
    {
        DiagnosticOwner.Computer => "computer",
        DiagnosticOwner.Browser => "browser",
        DiagnosticOwner.Portal => "portal",
        DiagnosticOwner.GovernmentAFirma => "government_afirma",
        _ => "unknown",
    };

    private static string StatusCode(DiagnosticStepStatus status) =>
        status switch
        {
            DiagnosticStepStatus.Success => "success",
            DiagnosticStepStatus.Failure => "failure",
            DiagnosticStepStatus.Skipped => "skipped",
            _ => "unknown",
        };

    private sealed record IncidentReport(
        string Schema,
        DateTimeOffset GeneratedAtUtc,
        IncidentCorrelation? Correlation,
        IncidentSummary Summary,
        IReadOnlyList<IncidentStep> Steps);

    private sealed record IncidentCorrelation(
        string? RequestId,
        string? TraceId,
        string? Action);

    private sealed record IncidentSummary(
        string Cause,
        string Owner,
        string OwnerLabel,
        string Responsibility,
        string SuggestedAction,
        string? FailureCode,
        string ExpertDetail);

    private sealed record IncidentStep(
        string Code,
        string Label,
        string Status,
        string Owner,
        string OwnerLabel,
        string Detail,
        string SuggestedAction);
}
