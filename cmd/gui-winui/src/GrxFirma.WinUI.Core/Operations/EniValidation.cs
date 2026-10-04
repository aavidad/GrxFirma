// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Operations;

public static class EniValidation
{
    private static bool Match(string text, string pattern) => Regex.IsMatch(text, pattern, RegexOptions.CultureInvariant);
    private static bool Safe(string text, int limit) => text.Length <= limit && !text.Any(char.IsControl);

    public static string? OrganError(string text)
    {
        if (!Safe(text, 128)) return "eni.validacion.dir3";
        var codes = text.Split(',');
        return codes.Length <= 128 && codes.All(code => Match(code.Trim().ToUpperInvariant(), @"\A[A-Z][0-9]{8}\z"))
            ? null : "eni.validacion.dir3";
    }

    public static string? IdentifierError(string text, bool required = false)
    {
        if (!Safe(text, 49)) return "eni.validacion.identifier";
        var value = text.Trim();
        if (value.Length == 0) return required ? "eni.validacion.source" : null;
        return Match(value, @"\AES_[A-Z][0-9]{8}_[0-9]{4}_[A-Za-z0-9_]{1,30}\z") ? null : "eni.validacion.identifier";
    }

    public static string? ClassificationError(string text) => Safe(text, 44) &&
        Match(text.Trim(), @"\A(?:[0-9]{1,30}|[A-Z][0-9]{8}_PRO_[A-Za-z0-9_]{1,30})\z")
        ? null : "eni.validacion.classification";

    public static string? FormatError(string text) => Safe(text, 32) &&
        (text.Trim().Length == 0 || Match(text.Trim(), @"\A[A-Za-z0-9.+-]{1,32}\z"))
        ? null : "eni.validacion.format";

    public static string? InterestedError(string text) => Safe(text, 256) &&
        text.Split(',').All(value => Safe(value.Trim(), 128)) ? null : "eni.validacion.text";

    public static string SerializeDate(DateTimeOffset? date, TimeSpan time)
    {
        if (date is null || time < TimeSpan.Zero || time >= TimeSpan.FromDays(1))
            throw new ArgumentException("eni.validacion.date");
        var local = new DateTime(date.Value.Year, date.Value.Month, date.Value.Day, 0, 0, 0, DateTimeKind.Unspecified).Add(time);
        if (TimeZoneInfo.Local.IsInvalidTime(local)) throw new ArgumentException("eni.validacion.date");
        return new DateTimeOffset(local, TimeZoneInfo.Local.GetUtcOffset(local))
            .ToString("yyyy-MM-ddTHH:mm:sszzz", CultureInfo.InvariantCulture);
    }

    public static string? DateError(DateTimeOffset? date, TimeSpan time)
    {
        try { _ = SerializeDate(date, time); return null; }
        catch (ArgumentException) { return "eni.validacion.date"; }
    }
}
