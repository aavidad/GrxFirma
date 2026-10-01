// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Ipc;

public sealed record IpcClientOptions
{
    public const int AbsoluteMaximumFrameBytes = 4 * 1024 * 1024;

    public int MaximumRequestBytes { get; init; } = 1024 * 1024;
    public int MaximumResponseBytes { get; init; } = AbsoluteMaximumFrameBytes;
    public TimeSpan ConnectTimeout { get; init; } = TimeSpan.FromSeconds(10);
    public TimeSpan OperationTimeout { get; init; } = TimeSpan.FromMinutes(5);

    internal void Validate()
    {
        if (MaximumRequestBytes is <= 0 or > AbsoluteMaximumFrameBytes ||
            MaximumResponseBytes is <= 0 or > AbsoluteMaximumFrameBytes)
        {
            throw IpcClientException.Configuration();
        }

        if (ConnectTimeout <= TimeSpan.Zero ||
            OperationTimeout <= TimeSpan.Zero ||
            ConnectTimeout > TimeSpan.FromMinutes(1) ||
            OperationTimeout > TimeSpan.FromMinutes(10))
        {
            throw IpcClientException.Configuration();
        }
    }
}

public sealed record IpcEndpoint(string PipePath, uint BackendProcessId);
