// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;

namespace GrxFirma.WinUI.Core.Operations;

// Fecha de una firma tal como la entrega el motor (RFC 3339 en UTC), escrita
// en la hora local con el formato del idioma. Si no se puede leer o el
// origen no es uno conocido, no se muestra nada: no se inventa una fecha.
public static class SigningTimeText
{
    public const string FromTimestamp = "timestamp";
    public const string FromSignedAttribute = "signed_attribute";

    public static bool TryFormat(
        string? raw,
        string? source,
        CultureInfo culture,
        TimeZoneInfo zone,
        out string text)
    {
        ArgumentNullException.ThrowIfNull(culture);
        ArgumentNullException.ThrowIfNull(zone);
        text = string.Empty;
        if (source is not (FromTimestamp or FromSignedAttribute) ||
            !DateTimeOffset.TryParseExact(
                raw ?? string.Empty,
                "yyyy-MM-dd'T'HH:mm:ssK",
                CultureInfo.InvariantCulture,
                DateTimeStyles.None,
                out var instant))
        {
            return false;
        }
        var local = TimeZoneInfo.ConvertTime(instant, zone);
        var pattern = culture.DateTimeFormat.ShortDatePattern;
        pattern = System.Text.RegularExpressions.Regex.Replace(pattern, "(?<!d)d(?!d)", "dd");
        pattern = System.Text.RegularExpressions.Regex.Replace(pattern, "(?<!M)M(?!M)", "MM");
        text = local.ToString(pattern + " " + culture.DateTimeFormat.LongTimePattern, culture);
        return true;
    }
}
