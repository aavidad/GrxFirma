// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;

namespace GrxFirma.WinUI.Core.Operations;

// El QR trae la fecha como DD-MM-AAAA y el importe con punto decimal. Para la
// persona se muestran con el formato del idioma de la aplicación; si el valor
// no tiene la forma esperada se enseña tal cual, sin inventar nada.
public static class VeriFactuQrDisplay
{
    public static string Date(string? raw, CultureInfo culture)
    {
        ArgumentNullException.ThrowIfNull(culture);
        var value = raw ?? string.Empty;
        return DateOnly.TryParseExact(value, "dd-MM-yyyy", CultureInfo.InvariantCulture,
            DateTimeStyles.None, out var date)
            ? date.ToString(TwoDigitShortDate(culture), culture)
            : value;
    }

    // Fecha corta del idioma con día y mes de dos cifras (01/09/2024): en
    // una factura se lee mejor que 1/9/2024.
    private static string TwoDigitShortDate(CultureInfo culture)
    {
        var pattern = culture.DateTimeFormat.ShortDatePattern;
        pattern = System.Text.RegularExpressions.Regex.Replace(pattern, "(?<!d)d(?!d)", "dd");
        return System.Text.RegularExpressions.Regex.Replace(pattern, "(?<!M)M(?!M)", "MM");
    }

    public static string Amount(string? raw, CultureInfo culture)
    {
        ArgumentNullException.ThrowIfNull(culture);
        var value = raw ?? string.Empty;
        if (!decimal.TryParse(value, NumberStyles.AllowLeadingSign | NumberStyles.AllowDecimalPoint,
            CultureInfo.InvariantCulture, out var amount)) return value;
        var format = (NumberFormatInfo)culture.NumberFormat.Clone();
        format.CurrencySymbol = "€";
        return amount.ToString("C2", format);
    }
}
