// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json.Serialization;
using System.Text.Json;
using GrxFirma.WinUI.Core.Diagnostics;

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Operations;

public sealed record PingParameters;

public sealed record PingResult;

public sealed record CheckCertificatesParameters;

public sealed record CheckCertificatesSummaryResult
{
    [JsonPropertyName("okCount")]
    public int OkCount { get; init; }

    [JsonPropertyName("failCount")]
    public int FailCount { get; init; }

    [JsonIgnore]
    public bool HasCoherentCounts =>
        OkCount >= 0 &&
        FailCount >= 0;
}

public sealed record CertificateAccessOptionsParameters;

// Los elementos no declaran los IDs, etiquetas, navegadores ni preferencias
// que devuelve el motor. System.Text.Json descarta esos campos y la interfaz
// solo conserva el número de opciones disponibles.
public sealed record CertificateAccessInventoryEntry;

public sealed record CertificateAccessInventoryResult
{
    [JsonPropertyName("managers")]
    [JsonConverter(typeof(CertificateAccessInventoryListConverter))]
    public IReadOnlyList<CertificateAccessInventoryEntry> Managers
    {
        get;
        init;
    } = [];

    [JsonPropertyName("importTargets")]
    [JsonConverter(typeof(CertificateAccessInventoryListConverter))]
    public IReadOnlyList<CertificateAccessInventoryEntry> ImportTargets
    {
        get;
        init;
    } = [];

    [JsonIgnore]
    public int VisibleManagerCount =>
        Math.Min(
            Managers?.Count ?? 0,
            OperationResultText.MaximumVisibleItems);

    [JsonIgnore]
    public int VisibleImportTargetCount =>
        Math.Min(
            ImportTargets?.Count ?? 0,
            OperationResultText.MaximumVisibleItems);
}

public sealed class CertificateAccessInventoryListConverter
    : JsonConverter<IReadOnlyList<CertificateAccessInventoryEntry>>
{
    public override IReadOnlyList<CertificateAccessInventoryEntry> Read(
        ref Utf8JsonReader reader,
        Type typeToConvert,
        JsonSerializerOptions options)
    {
        if (reader.TokenType != JsonTokenType.StartArray)
        {
            reader.Skip();
            return [];
        }

        var entries = new List<CertificateAccessInventoryEntry>(
            OperationResultText.MaximumVisibleItems);
        while (reader.Read() &&
            reader.TokenType != JsonTokenType.EndArray)
        {
            if (entries.Count < OperationResultText.MaximumVisibleItems)
            {
                entries.Add(new CertificateAccessInventoryEntry());
            }
            reader.Skip();
        }
        return entries;
    }

    public override void Write(
        Utf8JsonWriter writer,
        IReadOnlyList<CertificateAccessInventoryEntry> value,
        JsonSerializerOptions options)
    {
        writer.WriteStartArray();
        foreach (var _ in (value ?? [])
            .Take(OperationResultText.MaximumVisibleItems))
        {
            writer.WriteStartObject();
            writer.WriteEndObject();
        }
        writer.WriteEndArray();
    }
}

public sealed record ProxySecretStoreStatusParameters;

public sealed record ProxySecretStoreStatusResult
{
    private string _platform = string.Empty;
    private string _backend = string.Empty;
    private string _reason = string.Empty;
    private string _runtimeMode = string.Empty;

    [JsonPropertyName("available")]
    public bool Available { get; init; }

    [JsonPropertyName("platform")]
    public string Platform
    {
        get => _platform;
        init => _platform = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("backend")]
    public string Backend
    {
        get => _backend;
        init => _backend = OperationResultText.Clean(value, 160);
    }

    [JsonPropertyName("reason")]
    public string Reason
    {
        get => _reason;
        init => _reason = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("runtimeProxyMode")]
    public string RuntimeMode
    {
        get => _runtimeMode;
        init => _runtimeMode = OperationResultText.Clean(value, 80);
    }
}

public sealed record TlsDiagnosticsParameters;

public sealed record ClockDiagnosticsParameters;

public sealed record ExportDiagnosticParameters;

public sealed record InstallPublicRootsParameters;

public sealed record ClearTlsTrustParameters;

public sealed record TlsStoreDiagnosticResult
{
    private static readonly HashSet<string> AllowedStates =
        new(StringComparer.Ordinal)
        {
            "not_created",
            "empty",
            "available",
            "unavailable",
        };

    private string _state = string.Empty;

    [JsonPropertyName("state")]
    public string State
    {
        get => _state;
        init => _state = OperationResultText.Clean(value, 40);
    }

    [JsonPropertyName("artifactCount")]
    public int ArtifactCount { get; init; }

    [JsonPropertyName("certificateCount")]
    public int CertificateCount { get; init; }

    [JsonPropertyName("keyCount")]
    public int KeyCount { get; init; }

    [JsonIgnore]
    public bool IsCoherent =>
        AllowedStates.Contains(State) &&
        ArtifactCount >= 0 &&
        CertificateCount >= 0 &&
        KeyCount >= 0 &&
        CertificateCount <= ArtifactCount &&
        KeyCount <= ArtifactCount &&
        CertificateCount + KeyCount <= ArtifactCount;
}

public sealed record DesktopDiagnosticSummaryResult
{
    [JsonPropertyName("certificates")]
    public int CertificateCount { get; init; }

    [JsonPropertyName("canSign")]
    public int CanSignCount { get; init; }

    [JsonPropertyName("tlsStore")]
    public TlsStoreDiagnosticResult? TlsStore { get; init; }

    [JsonIgnore]
    public bool IsCoherent =>
        CertificateCount >= 0 &&
        CanSignCount >= 0 &&
        CanSignCount <= CertificateCount &&
        TlsStore is { IsCoherent: true };
}

public sealed record ClockDiagnosticStepResult
{
    private string _code = string.Empty;
    private string _label = string.Empty;
    private string _owner = string.Empty;
    private string _userMessage = string.Empty;
    private string _suggestedAction = string.Empty;
    private string _evidenceRef = string.Empty;

    [JsonPropertyName("code")]
    public string Code
    {
        get => _code;
        init => _code = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("label")]
    public string Label
    {
        get => _label;
        init => _label = OperationResultText.Clean(
            value,
            160,
            CatalogLocalizer.Shared.TranslateVisibleText("Fase de fecha y hora"));
    }

    [JsonPropertyName("status")]
    [JsonConverter(typeof(DiagnosticStepStatusJsonConverter))]
    public DiagnosticStepStatus Status { get; init; }

    [JsonPropertyName("owner")]
    public string Owner
    {
        get => _owner;
        init => _owner = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("userMessage")]
    public string UserMessage
    {
        get => _userMessage;
        init => _userMessage = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("suggestedAction")]
    public string SuggestedAction
    {
        get => _suggestedAction;
        init => _suggestedAction =
            OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("evidenceRef")]
    public string EvidenceRef
    {
        get => _evidenceRef;
        init => _evidenceRef =
            OperationResultText.Clean(value, 160);
    }

    public OperationDiagnosticStep ToOperationStep() => new()
    {
        Code = Code,
        Label = Label,
        Status = Status,
        Owner = Owner,
        UserMessage = UserMessage,
        SuggestedAction = SuggestedAction,
        EvidenceRef = string.IsNullOrWhiteSpace(EvidenceRef)
            ? null
            : EvidenceRef,
    };
}

public sealed record ClockDiagnosticResult
{
    private static readonly HashSet<string> ExpectedCodes =
        new(StringComparer.Ordinal)
        {
            "local_clock",
            "remote_clock",
            "government_afirma",
        };

    [JsonPropertyName("thresholdSeconds")]
    public int ThresholdSeconds { get; init; }

    [JsonPropertyName("steps")]
    public IReadOnlyList<ClockDiagnosticStepResult> Steps
    {
        get;
        init;
    } = [];

    [JsonIgnore]
    public bool IsCoherent =>
        ThresholdSeconds is > 0 and <= 60 &&
        Steps is { Count: 3 } &&
        Steps.All(step =>
            step is not null &&
            ExpectedCodes.Contains(step.Code)) &&
        Steps.Select(step => step.Code)
            .Distinct(StringComparer.Ordinal)
            .Count() == ExpectedCodes.Count;
}
