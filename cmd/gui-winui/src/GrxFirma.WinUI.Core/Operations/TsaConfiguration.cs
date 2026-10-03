// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class TsaConfiguration
{
    public static bool TryNormalize(bool enabled, string? value, out string url)
    {
        url = value?.Trim() ?? string.Empty;
        if (!enabled) return true;
        if (url.Length is 0 or > 2048 || url.Any(char.IsControl) ||
            url.Any(char.IsWhiteSpace) ||
            !Uri.TryCreate(url, UriKind.Absolute, out var parsed) ||
            parsed.Scheme != Uri.UriSchemeHttps ||
            string.IsNullOrWhiteSpace(parsed.Host) ||
            parsed.UserInfo.Length != 0 || parsed.Fragment.Length != 0)
        {
            return false;
        }
        return true;
    }
}
