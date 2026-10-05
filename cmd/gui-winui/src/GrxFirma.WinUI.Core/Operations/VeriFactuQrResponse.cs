// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text;
using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Operations;

// Traduce la respuesta JSON del cotejo de la AEAT a la clave de una frase.
// La AEAT no publica un esquema estable de esa respuesta: solo se afirma un
// resultado cuando el texto es inequívoco; si no, se dice que no se reconoce
// y la pantalla ofrece la respuesta técnica. Mismas reglas que Qt
// (cmd/gui-qml/qml/VeriFactuResponse.js).
public static class VeriFactuQrResponse
{
    public const string FoundKey = "verifactu.qr_aeat_found";
    public const string NotFoundKey = "verifactu.qr_aeat_not_found";
    public const string UnknownKey = "verifactu.qr_aeat_unknown";

    private static readonly Regex NegativeAnswer = new Regex(@"\bno\s*(?:se\s*)?(?:ha\s*)?(?:encontrad|encuentr|consta|existe|registrad|identificad)|noencontrad|not[\s_-]*found|incorrect|""ko""", RegexOptions.CultureInvariant, TimeSpan.FromMilliseconds(250));
    private static readonly Regex PositiveAnswer = new Regex(@"encontrad|correct|consta|registrad|identificad|""ok""|""found""", RegexOptions.CultureInvariant, TimeSpan.FromMilliseconds(250));

    public static string Classify(string? json)
    {
        var text = Normalize(json);
        try
        {
            if (NegativeAnswer.IsMatch(text)) return NotFoundKey;
            if (PositiveAnswer.IsMatch(text)) return FoundKey;
        }
        catch (RegexMatchTimeoutException)
        {
            return UnknownKey;
        }
        return UnknownKey;
    }

    private static string Normalize(string? json)
    {
        var decomposed = (json ?? string.Empty).ToLowerInvariant().Normalize(NormalizationForm.FormD);
        var builder = new StringBuilder(decomposed.Length);
        foreach (var character in decomposed)
        {
            if (CharUnicodeInfo.GetUnicodeCategory(character) != UnicodeCategory.NonSpacingMark)
                builder.Append(character);
        }
        return builder.ToString();
    }
}
