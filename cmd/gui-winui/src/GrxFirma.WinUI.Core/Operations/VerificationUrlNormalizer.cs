// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;

namespace GrxFirma.WinUI.Core.Operations;

public static class VerificationUrlNormalizer
{
    public static bool TryNormalize(string? raw, out string normalized)
    {
        normalized = string.Empty;
        var value = raw?.Trim() ?? string.Empty;
        if (value.Length == 0 || value.Length > 2048 ||
            value.Any(c => char.IsWhiteSpace(c) || char.IsControl(c) || c == '\\'))
            return false;
        if (!value.Contains("://", StringComparison.Ordinal))
        {
            if (value.Contains(':')) return false;
            value = "https://" + value;
        }
        if (!value.StartsWith("https://", StringComparison.OrdinalIgnoreCase)) return false;

        var end = value.IndexOfAny(new[] { '/', '?', '#' }, 8);
        if (end < 0) end = value.Length;
        var authority = value[8..end];
        if (authority.Length == 0 || authority.Contains('@') || authority.Contains('%')) return false;
        var host = authority;
        var port = string.Empty;
        if (authority.StartsWith('['))
        {
            var close = authority.IndexOf(']');
            if (close < 0) return false;
            host = authority[..(close + 1)];
            port = authority[(close + 1)..];
        }
        else
        {
            var colon = authority.LastIndexOf(':');
            if (colon >= 0) { host = authority[..colon]; port = authority[colon..]; }
        }
        if (port.Length > 0 && (port[0] != ':' || port.Length is < 2 or > 6 ||
            port[1..].Any(c => c is < '0' or > '9') ||
            !int.TryParse(port[1..], NumberStyles.None, CultureInfo.InvariantCulture, out var number) ||
            number is < 1 or > 65535)) return false;
        if (!Uri.TryCreate(value, UriKind.Absolute, out var uri) ||
            uri.Scheme != Uri.UriSchemeHttps || uri.Host.Length == 0 || uri.UserInfo.Length != 0)
            return false;
        if (uri.HostNameType != UriHostNameType.IPv6)
        {
            try
            {
                var idn = new IdnMapping { UseStd3AsciiRules = true };
                host = idn.GetAscii(host).ToLowerInvariant();
                // Validar también la entrada ACE, sin confiar en su prefijo xn--.
                var unicode = idn.GetUnicode(host);
                if (idn.GetAscii(unicode) != host) return false;
                // IdnMapping con NLS (Windows) no aplica la regla Bidi de IDNA
                // (RFC 5893); el motor Go sí. Se comprueba aquí para rechazar lo mismo.
                if (unicode.Split('.').Any(label => !CumpleReglaBidi(label))) return false;
            }
            catch (ArgumentException) { return false; }
            if (host.Length > 253 || host.Split('.').Any(label =>
                label.Length is 0 or > 63 || label[0] == '-' || label[^1] == '-' ||
                label.Any(c => !(c is >= 'a' and <= 'z' or >= '0' and <= '9' or '-'))))
                return false;
        }
        else if (!host.StartsWith('[')) return false;

        var result = "https://" + host + port + value[end..];
        if (result.Length > 2048) return false;
        normalized = result;
        return true;
    }

    // Una etiqueta con escritura de derecha a izquierda debe empezar por una
    // letra de esa escritura y no contener letras de izquierda a derecha.
    private static bool CumpleReglaBidi(string label)
    {
        if (!label.Any(EsDerechaAIzquierda)) return true;
        if (!EsDerechaAIzquierda(label[0])) return false;
        return !label.Any(c => char.IsLetter(c) && !EsDerechaAIzquierda(c));
    }

    private static bool EsDerechaAIzquierda(char c) =>
        c is >= '\u0590' and <= '\u08FF' or >= '\uFB1D' and <= '\uFDFF' or >= '\uFE70' and <= '\uFEFF';
}
