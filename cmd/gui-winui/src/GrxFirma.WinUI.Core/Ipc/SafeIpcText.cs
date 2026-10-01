// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text;

namespace GrxFirma.WinUI.Core.Ipc;

public static class SafeIpcText
{
    public static string Clean(
        string? value,
        int maximumCharacters,
        string fallback)
    {
        if (maximumCharacters <= 0)
        {
            return fallback;
        }

        var trimmed = value?.Trim();
        if (string.IsNullOrEmpty(trimmed))
        {
            return fallback;
        }

        var output = new StringBuilder(Math.Min(trimmed.Length, maximumCharacters));
        foreach (var rune in trimmed.EnumerateRunes())
        {
            if (output.Length + rune.Utf16SequenceLength > maximumCharacters)
            {
                break;
            }

            if (Rune.GetUnicodeCategory(rune) == UnicodeCategory.Format)
            {
                continue;
            }

            if (Rune.IsControl(rune) &&
                rune.Value is not ('\r' or '\n' or '\t'))
            {
                continue;
            }

            output.Append(rune.ToString());
        }

        var cleaned = output.ToString().Trim();
        return cleaned.Length == 0 ? fallback : cleaned;
    }
}
