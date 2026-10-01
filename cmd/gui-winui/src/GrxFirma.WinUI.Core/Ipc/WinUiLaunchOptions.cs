// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;

namespace GrxFirma.WinUI.Core.Ipc;

public sealed record WinUiLaunchOptions(string? IpcSocket, uint? BackendProcessId, bool StartHidden = false)
{
    public bool HasBackendEndpoint =>
        !string.IsNullOrWhiteSpace(IpcSocket) && BackendProcessId is > 0;

    public IpcEndpoint ToEndpoint()
    {
        if (!HasBackendEndpoint)
        {
            throw IpcClientException.Configuration();
        }

        return new IpcEndpoint(IpcSocket!, BackendProcessId!.Value);
    }

    public static bool TryParse(
        IReadOnlyList<string> arguments,
        out WinUiLaunchOptions options,
        out string safeError)
    {
        string? socket = null;
        uint? backendProcessId = null;
        var startHidden = false;

        for (var index = 0; index < arguments.Count; index++)
        {
            var argument = arguments[index];
            if (argument == "--start-hidden" && !startHidden)
            {
                startHidden = true;
                continue;
            }
            if (TryReadOption(arguments, ref index, argument, "--ipc-socket", out var socketValue))
            {
                if (socket is not null || string.IsNullOrWhiteSpace(socketValue))
                {
                    return Fail(out options, out safeError);
                }

                socket = socketValue;
                continue;
            }

            if (TryReadOption(arguments, ref index, argument, "--backend-pid", out var pidValue))
            {
                if (backendProcessId is not null ||
                    !uint.TryParse(
                        pidValue,
                        NumberStyles.None,
                        CultureInfo.InvariantCulture,
                        out var parsedProcessId) ||
                    parsedProcessId == 0)
                {
                    return Fail(out options, out safeError);
                }

                backendProcessId = parsedProcessId;
                continue;
            }

            return Fail(out options, out safeError);
        }

        if ((socket is null) != (backendProcessId is null))
        {
            return Fail(out options, out safeError);
        }

        options = new WinUiLaunchOptions(socket, backendProcessId, startHidden);
        safeError = string.Empty;
        return true;
    }

    private static bool TryReadOption(
        IReadOnlyList<string> arguments,
        ref int index,
        string argument,
        string option,
        out string value)
    {
        value = string.Empty;
        if (string.Equals(argument, option, StringComparison.Ordinal))
        {
            if (++index >= arguments.Count)
            {
                return true;
            }

            value = arguments[index];
            return true;
        }

        var prefix = option + "=";
        if (argument.StartsWith(prefix, StringComparison.Ordinal))
        {
            value = argument[prefix.Length..];
            return true;
        }

        return false;
    }

    private static bool Fail(
        out WinUiLaunchOptions options,
        out string safeError)
    {
        options = new WinUiLaunchOptions(null, null);
        safeError = "Los parámetros de inicio de la interfaz no son válidos.";
        return false;
    }
}
