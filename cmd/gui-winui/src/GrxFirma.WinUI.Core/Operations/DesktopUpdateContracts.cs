// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Operations;

public sealed record UpdateCheckParameters;

public sealed record UpdateCheckResult
{
    [JsonPropertyName("version_actual")]
    public string CurrentVersion { get; init; } = string.Empty;

    [JsonPropertyName("ultima_version")]
    public string LatestVersion { get; init; } = string.Empty;

    [JsonPropertyName("hay_nueva")]
    public bool HasNewVersion { get; init; }

    [JsonPropertyName("comparable")]
    public bool IsComparable { get; init; }

    [JsonPropertyName("url")]
    public string ReleaseUrl { get; init; } = string.Empty;

    [JsonPropertyName("estado")]
    public string State { get; init; } = string.Empty;

    [JsonPropertyName("mensaje")]
    public string Message { get; init; } = string.Empty;

    [JsonPropertyName("titulo")]
    public string Title { get; init; } = string.Empty;
}
