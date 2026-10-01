// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Ipc;

public enum IpcClientEventKind
{
    Connecting,
    Connected,
    RequestStarted,
    RequestCompleted,
    RequestFailed,
    Disconnected,
}

// Este evento contiene exclusivamente metadatos de correlación. Nunca acepta
// params, data, rutas, secretos ni mensajes de excepción.
public sealed record IpcClientEvent(
    DateTimeOffset Timestamp,
    IpcClientEventKind Kind,
    string Action,
    string RequestId,
    string TraceId,
    string? ErrorCode = null);

public interface IIpcEventSink
{
    void Write(IpcClientEvent clientEvent);
}

public sealed class NullIpcEventSink : IIpcEventSink
{
    public static NullIpcEventSink Instance { get; } = new();

    private NullIpcEventSink()
    {
    }

    public void Write(IpcClientEvent clientEvent)
    {
    }
}
