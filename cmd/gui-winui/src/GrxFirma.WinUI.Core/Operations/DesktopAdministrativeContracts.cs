// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Operations;

public sealed class InvoiceValidationResult
{
    [JsonPropertyName("format")] public string Format { get; init; } = "";
    [JsonPropertyName("valid")] public bool Valid { get; init; }
    [JsonPropertyName("errors")] public int Errors { get; init; }
    [JsonPropertyName("warnings")] public int Warnings { get; init; }
    [JsonPropertyName("issues")] public IReadOnlyList<InvoiceValidationIssue> Issues { get; init; } = [];
    [JsonPropertyName("report")] public string Report { get; init; } = "";
}

public sealed class InvoiceValidationIssue
{
    [JsonPropertyName("level")] public string Level { get; init; } = "";
    [JsonPropertyName("field")] public string Field { get; init; } = "";
    [JsonPropertyName("message")] public string Message { get; init; } = "";
}

public sealed class EniGenerationResult
{
    [JsonPropertyName("outputPath")] public string OutputPath { get; init; } = "";
    [JsonPropertyName("documents")] public int Documents { get; init; }
}
