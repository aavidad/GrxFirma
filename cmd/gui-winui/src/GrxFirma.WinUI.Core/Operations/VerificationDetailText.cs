// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.RegularExpressions;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>
/// Traduce los detalles y tipos de evidencia del verificador con el catálogo
/// compartido, igual que informeverificacion.TraducirDetalle (motor) y
/// verificationDetailText (Qt): «error_cadena=x509_caducado» pasa a
/// «Cadena del certificado: un certificado de la cadena está caducado…».
/// Lo que el catálogo no conoce se deja tal cual.
/// </summary>
public static partial class VerificationDetailText
{
    // Prefijos de las familias de claves del catálogo; el sufijo lo pone el motor.
    private const string DetailPrefix = "verificacion.detalle.";
    private const string ValuePrefix = "verificacion.detalle.valor.";
    private const string EvidencePrefix = "report.evidence.";
    private const string FormatKey = "verificacion.detalle.formato";

    [GeneratedRegex(@"^cobertura_firma_pdf_(\d{1,4})$", RegexOptions.CultureInvariant)]
    private static partial Regex PdfCoverage();

    [GeneratedRegex(@"^signer\[(\d{1,4})\]\.chain_length$", RegexOptions.CultureInvariant)]
    private static partial Regex SignerChain();

    [GeneratedRegex(@"^revision_hasta_(\d{1,12})_de_(\d{1,12})$", RegexOptions.CultureInvariant)]
    private static partial Regex RevisionUntil();

    [GeneratedRegex(@"^cert\[\d+\] (.+): (bueno|revocado|desconocido)(?: vía (\w+))?(?: \((.*)\))?$",
        RegexOptions.CultureInvariant | RegexOptions.Singleline, 50)]
    private static partial Regex Revocation();

    public static string Detail(string? line) => Detail(CatalogLocalizer.Shared, line);

    public static string Detail(CatalogLocalizer localizer, string? line)
    {
        ArgumentNullException.ThrowIfNull(localizer);
        var text = (line ?? string.Empty).Trim();
        if (text.Length == 0) return text;

        Match revocation;
        try { revocation = Revocation().Match(text); }
        catch (RegexMatchTimeoutException) { revocation = Match.Empty; }
        if (revocation.Success)
        {
            var state = localizer.Text(revocation.Groups[2].Value switch
            {
                "bueno" => "verificacion.detalle.revocacion.bueno",
                "revocado" => "verificacion.detalle.revocacion.revocado",
                _ => "verificacion.detalle.revocacion.desconocido",
            });
            if (revocation.Groups[3].Success)
                state += " (" + revocation.Groups[3].Value.ToUpperInvariant() + ")";
            return Format(localizer, FormatKey, revocation.Groups[1].Value, state);
        }

        var cut = text.IndexOf('=');
        if (cut <= 0 || text.AsSpan(0, cut).IndexOfAny(' ', '\t') >= 0)
            return localizer.TranslateVisibleText(text);
        var key = text[..cut];
        var value = text[(cut + 1)..];

        string label;
        Match match;
        if ((match = PdfCoverage().Match(key)).Success)
        {
            label = Format(localizer, "verificacion.detalle.cobertura_firma_pdf", match.Groups[1].Value);
        }
        else if ((match = SignerChain().Match(key)).Success)
        {
            // «signer[0].chain_length=3»: firmantes numerados desde 1.
            var index = long.Parse(match.Groups[1].Value, CultureInfo.InvariantCulture);
            label = Format(localizer, "verificacion.detalle.cadena_firmante",
                (index + 1).ToString(CultureInfo.InvariantCulture));
        }
        else if (Known(localizer, DetailPrefix + key, out var translated))
        {
            label = translated;
        }
        else
        {
            return text;
        }
        return Format(localizer, FormatKey, label, Value(localizer, value));
    }

    /// <summary>«certificate.subject» → «Titular del certificado».</summary>
    public static string EvidenceType(string? type) => EvidenceType(CatalogLocalizer.Shared, type);

    public static string EvidenceType(CatalogLocalizer localizer, string? type)
    {
        ArgumentNullException.ThrowIfNull(localizer);
        var text = (type ?? string.Empty).Trim();
        if (text.Length == 0) return text;
        return Known(localizer, EvidencePrefix + text, out var translated)
            ? translated
            : localizer.TranslateVisibleText(text);
    }

    private static string Value(CatalogLocalizer localizer, string value)
    {
        var revision = RevisionUntil().Match(value);
        if (revision.Success)
            return Format(localizer, "verificacion.detalle.valor.revision_hasta",
                revision.Groups[1].Value, revision.Groups[2].Value);
        return value.Length > 0 && Known(localizer, ValuePrefix + value, out var translated)
            ? translated
            : value;
    }

    private static bool Known(CatalogLocalizer localizer, string key, out string translated)
    {
        translated = localizer.Text(key);
        return !string.Equals(translated, key, StringComparison.Ordinal);
    }

    // Los textos del motor usan %s y %d (fmt de Go); se sustituyen en orden.
    private static string Format(CatalogLocalizer localizer, string key, params string[] values)
    {
        var template = localizer.Text(key);
        var output = new System.Text.StringBuilder(template.Length + 32);
        var index = 0;
        for (var position = 0; position < template.Length; position++)
        {
            if (template[position] == '%' && position + 1 < template.Length &&
                (template[position + 1] is 's' or 'd') && index < values.Length)
            {
                output.Append(values[index++]);
                position++;
                continue;
            }
            output.Append(template[position]);
        }
        return output.ToString();
    }
}
