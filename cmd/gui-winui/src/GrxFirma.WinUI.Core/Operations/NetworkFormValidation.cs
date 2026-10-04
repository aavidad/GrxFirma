// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class NetworkFormValidation
{
    public static string? TsaError(bool enabled, string? url) =>
        TsaConfiguration.TryNormalize(enabled, url, out _) ? null : "winui.parity.tsa.invalid";

    public static string? ProxyPortError(string? text) =>
        int.TryParse(text, out var port) && port is >= 1 and <= 65535
            ? null : "validacion.proxy.puerto";

    public static string? ProxyHostError(bool enabled, string? type, string? value)
    {
        if (!enabled || type != "manual") return null;
        var host = value?.Trim() ?? string.Empty;
        return host.Length is >= 1 and <= 512 &&
            !host.Any(char.IsControl) && !host.Any(char.IsWhiteSpace) &&
            !host.Contains("://", StringComparison.Ordinal) &&
            host.IndexOfAny(['/', '\\', '@']) < 0
            ? null : "validacion.proxy.host";
    }
}
