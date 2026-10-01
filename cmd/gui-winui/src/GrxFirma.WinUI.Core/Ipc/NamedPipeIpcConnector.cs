// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Security.Principal;
using Microsoft.Win32.SafeHandles;

namespace GrxFirma.WinUI.Core.Ipc;

public sealed class NamedPipeIpcConnector : IIpcConnector
{
    private const string LocalPipePrefix = @"\\.\pipe\";

    public async Task<Stream> ConnectAsync(
        IpcEndpoint endpoint,
        IpcClientOptions options,
        CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(endpoint);
        ArgumentNullException.ThrowIfNull(options);
        options.Validate();

        if (!OperatingSystem.IsWindows() ||
            endpoint.BackendProcessId == 0 ||
            !TryGetLocalPipeName(endpoint.PipePath, out var pipeName))
        {
            throw IpcClientException.Configuration();
        }

        var pipe = new NamedPipeClientStream(
            ".",
            pipeName,
            PipeDirection.InOut,
            PipeOptions.Asynchronous | PipeOptions.WriteThrough,
            TokenImpersonationLevel.Anonymous,
            HandleInheritability.None);

        try
        {
            var timeoutMilliseconds = checked((int)options.ConnectTimeout.TotalMilliseconds);
            await pipe.ConnectAsync(timeoutMilliseconds, cancellationToken).ConfigureAwait(false);

            if (!GetNamedPipeServerProcessId(pipe.SafePipeHandle, out var actualProcessId) ||
                actualProcessId != endpoint.BackendProcessId)
            {
                pipe.Dispose();
                throw IpcClientException.ServerIdentity();
            }

            pipe.ReadMode = PipeTransmissionMode.Byte;
            return pipe;
        }
        catch (IpcClientException)
        {
            pipe.Dispose();
            throw;
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            pipe.Dispose();
            throw;
        }
        catch
        {
            pipe.Dispose();
            throw IpcClientException.Connection();
        }
    }

    internal static bool TryGetLocalPipeName(string? path, out string pipeName)
    {
        pipeName = string.Empty;
        if (string.IsNullOrWhiteSpace(path) ||
            !path.StartsWith(LocalPipePrefix, StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        var candidate = path[LocalPipePrefix.Length..];
        if (candidate.Length is 0 or > 200 ||
            candidate.Contains('\\') ||
            candidate.Contains('/') ||
            candidate.Any(char.IsControl))
        {
            return false;
        }

        pipeName = candidate;
        return true;
    }

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetNamedPipeServerProcessId(
        SafePipeHandle pipe,
        out uint serverProcessId);
}
