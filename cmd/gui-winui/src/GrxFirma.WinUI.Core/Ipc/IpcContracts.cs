// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Diagnostics;

namespace GrxFirma.WinUI.Core.Ipc;

public static class DesktopIpcProtocol
{
    public const string Name = "desktop-ipc-v1";
    public const string Version = "1.0";
    public const string HelloAction = "hello";

    public static IReadOnlySet<string> RequiredCapabilities { get; } =
        new HashSet<string>(StringComparer.Ordinal)
        {
            "certificate-id-selection",
            "correlation",
            "guided-diagnostic-steps",
            "stable-result-metadata",
        };
}

internal sealed record IpcWireRequest<TParameters>
{
    [JsonPropertyName("protocol")]
    public required string Protocol { get; init; }

    [JsonPropertyName("requestId")]
    public required string RequestId { get; init; }

    [JsonPropertyName("traceId")]
    public required string TraceId { get; init; }

    [JsonPropertyName("action")]
    public required string Action { get; init; }

    [JsonPropertyName("params")]
    public required TParameters Parameters { get; init; }
}

internal sealed record IpcWireResponse<TData>
{
    [JsonPropertyName("protocol")]
    public string? Protocol { get; init; }

    [JsonPropertyName("requestId")]
    public string? RequestId { get; init; }

    [JsonPropertyName("traceId")]
    public string? TraceId { get; init; }

    [JsonPropertyName("ok")]
    public bool? Ok { get; init; }

    [JsonPropertyName("action")]
    public string? Action { get; init; }

    [JsonPropertyName("errorCode")]
    public string? ErrorCode { get; init; }

    [JsonPropertyName("phase")]
    public string? Phase { get; init; }

    [JsonPropertyName("retryable")]
    public bool? Retryable { get; init; }

    [JsonPropertyName("outcome")]
    public string? Outcome { get; init; }

    [JsonPropertyName("data")]
    public TData? Data { get; init; }

    // Se deserializa para consumir el frame completo, pero nunca se propaga
    // ni se registra: el backend puede incluir rutas u otros detalles.
    [JsonPropertyName("error")]
    public string? UnsafeError { get; init; }

    [JsonPropertyName("diagnostic")]
    public OperationDiagnostic? Diagnostic { get; init; }
}

public sealed record IpcCallResult<TData>
{
    public required string Protocol { get; init; }
    public required string RequestId { get; init; }
    public required string TraceId { get; init; }
    public required string Action { get; init; }
    public required bool IsSuccess { get; init; }
    public required string Outcome { get; init; }
    public string? ErrorCode { get; init; }
    public string? Phase { get; init; }
    public bool Retryable { get; init; }
    public TData? Data { get; init; }
    public OperationDiagnostic? Diagnostic { get; init; }

    public string SafeUserMessage =>
        IsSuccess
            ? "Operación completada."
            : SafeIpcText.Clean(
                Diagnostic?.UserMessage,
                512,
                "La operación no se pudo completar.");
}

public sealed record IpcHello
{
    [JsonPropertyName("protocol")]
    public string Protocol { get; init; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; init; } = string.Empty;

    [JsonPropertyName("build")]
    public IpcHelloBuild Build { get; init; } = new();

    [JsonPropertyName("capabilities")]
    public IReadOnlyList<string> Capabilities { get; init; } = [];

    [JsonPropertyName("actions")]
    public IReadOnlyList<string> Actions { get; init; } = [];

    [JsonPropertyName("limits")]
    public IpcHelloLimits Limits { get; init; } = new();

    [JsonPropertyName("idleTimeoutSeconds")]
    public long IdleTimeoutSeconds { get; init; }
}

public sealed record IpcHelloBuild
{
    [JsonPropertyName("version")]
    public string Version { get; init; } = string.Empty;

    [JsonPropertyName("target")]
    public string Target { get; init; } = string.Empty;
}

public sealed record IpcHelloLimits
{
    [JsonPropertyName("maxRequestBytes")]
    public int MaxRequestBytes { get; init; }

    [JsonPropertyName("maxConcurrentConnections")]
    public int MaxConcurrentConnections { get; init; }

    [JsonPropertyName("defaultTimeoutSeconds")]
    public long DefaultTimeoutSeconds { get; init; }

    [JsonPropertyName("longOperationTimeoutSeconds")]
    public long LongOperationTimeoutSeconds { get; init; }
}

internal sealed record EmptyIpcParameters;
