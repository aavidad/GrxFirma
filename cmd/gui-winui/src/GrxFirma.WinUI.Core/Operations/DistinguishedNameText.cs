// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text;

namespace GrxFirma.WinUI.Core.Operations;

// Nombre legible de un titular: el CN de un nombre distinguido (RFC 4514,
// con comas escapadas). Si no hay CN, el texto recibido tal cual.
public static class DistinguishedNameText
{
    public static string CommonName(string? distinguishedName)
    {
        var text = (distinguishedName ?? string.Empty).Trim();
        foreach (var component in Components(text))
        {
            var equals = component.IndexOf('=');
            if (equals <= 0) continue;
            var attribute = component[..equals].Trim();
            if (!attribute.Equals("CN", StringComparison.OrdinalIgnoreCase) &&
                !attribute.Equals("2.5.4.3", StringComparison.Ordinal)) continue;
            var value = component[(equals + 1)..].Trim();
            if (value.Length > 0 && !value.StartsWith('#')) return value;
        }
        return text;
    }

    private static IEnumerable<string> Components(string text)
    {
        var current = new StringBuilder();
        for (var i = 0; i < text.Length; i++)
        {
            var character = text[i];
            if (character == '\\' && i + 1 < text.Length)
            {
                current.Append(text[++i]);
                continue;
            }
            if (character is ',' or ';' or '+')
            {
                yield return current.ToString();
                current.Clear();
                continue;
            }
            current.Append(character);
        }
        yield return current.ToString();
    }
}
