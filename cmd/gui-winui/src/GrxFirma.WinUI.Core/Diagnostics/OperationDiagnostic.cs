// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Diagnostics;

public enum DiagnosticStepStatus
{
    Unknown,
    Success,
    Failure,
    Skipped,
}

public sealed record OperationDiagnostic
{
    // La correlación se añade únicamente después de validar el sobre IPC. No
    // se acepta desde el objeto "diagnostic" enviado por el backend y por eso
    // queda fuera de su contrato JSON.
    [JsonIgnore]
    public string? RequestId { get; init; }

    [JsonIgnore]
    public string? TraceId { get; init; }

    [JsonIgnore]
    public string? Action { get; init; }

    [JsonPropertyName("category")]
    public string Category { get; init; } = "unknown";

    [JsonPropertyName("failureCode")]
    public string? FailureCode { get; init; }

    [JsonPropertyName("userMessage")]
    public string UserMessage { get; init; } = string.Empty;

    [JsonPropertyName("expertMessage")]
    public string ExpertMessage { get; init; } = string.Empty;

    [JsonPropertyName("likelyOwner")]
    public string LikelyOwner { get; init; } = "unknown";

    [JsonPropertyName("responsibilityMessage")]
    public string ResponsibilityMessage { get; init; } = string.Empty;

    [JsonPropertyName("suggestedAction")]
    public string SuggestedAction { get; init; } = string.Empty;

    [JsonPropertyName("userCanResolveDirectly")]
    public bool UserCanResolveDirectly { get; init; }

    [JsonPropertyName("steps")]
    public IReadOnlyList<OperationDiagnosticStep> Steps { get; init; } = [];
}

public sealed record OperationDiagnosticStep
{
    [JsonPropertyName("code")]
    public string Code { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    [JsonPropertyName("status")]
    [JsonConverter(typeof(DiagnosticStepStatusJsonConverter))]
    public DiagnosticStepStatus Status { get; init; } = DiagnosticStepStatus.Unknown;

    [JsonPropertyName("owner")]
    public string? Owner { get; init; }

    [JsonPropertyName("userMessage")]
    public string? UserMessage { get; init; }

    [JsonPropertyName("suggestedAction")]
    public string? SuggestedAction { get; init; }

    [JsonPropertyName("evidenceRef")]
    public string? EvidenceRef { get; init; }
}

public sealed class DiagnosticStepStatusJsonConverter
    : JsonConverter<DiagnosticStepStatus>
{
    public override DiagnosticStepStatus Read(
        ref Utf8JsonReader reader,
        Type typeToConvert,
        JsonSerializerOptions options)
    {
        if (reader.TokenType != JsonTokenType.String)
        {
            return DiagnosticStepStatus.Unknown;
        }

        return reader.GetString()?.Trim().ToLowerInvariant() switch
        {
            "success" => DiagnosticStepStatus.Success,
            "failure" => DiagnosticStepStatus.Failure,
            "skipped" => DiagnosticStepStatus.Skipped,
            _ => DiagnosticStepStatus.Unknown,
        };
    }

    public override void Write(
        Utf8JsonWriter writer,
        DiagnosticStepStatus value,
        JsonSerializerOptions options)
    {
        writer.WriteStringValue(value switch
        {
            DiagnosticStepStatus.Success => "success",
            DiagnosticStepStatus.Failure => "failure",
            DiagnosticStepStatus.Skipped => "skipped",
            _ => "unknown",
        });
    }
}
