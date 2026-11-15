// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>Un apartado de la ayuda larga: su título y su texto.</summary>
public sealed record UnsafeHelpSection(string Title, string Body);

/// <summary>
/// Presentación de los textos de una excepción insegura (lote 39h). No hay
/// ningún texto para personas: solo se trocean, eligen y ordenan claves y
/// textos del catálogo. Qt usa la misma lógica (cmd/gui-qml/qml/UnsafeText.js).
/// </summary>
public static partial class UnsafeExceptionText
{
    /// <summary>Formato de fecha del motor si el catálogo no trae el suyo.</summary>
    public const string DefaultDateLayout = "02/01/2006 15:04";

    // Con el caso fijado por la organización, la ayuda no explica cómo
    // activarlo ni deshacerlo: quedan los cuatro primeros apartados (qué hace,
    // por qué es inseguro, cuándo tiene sentido y la alternativa) y el de la
    // organización.
    public const int SectionsKeptForOrganization = 4;

    private const int MaximumTitleLength = 80;

    [GeneratedRegex(@"\r?\n[ \t]*\r?\n", RegexOptions.CultureInvariant)]
    private static partial Regex BlankLine();

    [GeneratedRegex("2006|01|02|15|04|05", RegexOptions.CultureInvariant)]
    private static partial Regex LayoutToken();

    /// <summary>«Qué hace este ajuste: mientras…» → título y texto. Admite los
    /// dos puntos de ancho completo del chino y el espacio previo del francés.</summary>
    public static UnsafeHelpSection SplitSection(string? text)
    {
        var clean = (text ?? string.Empty).Trim();
        var cut = -1;
        for (var i = 0; i < clean.Length && i <= MaximumTitleLength; i++)
        {
            if (clean[i] is ':' or '：') { cut = i; break; }
        }
        if (cut <= 0) return new(string.Empty, clean);
        var body = clean[(cut + 1)..].Trim();
        if (body.Length > 0)
            body = char.ToUpperInvariant(body[0]) + body[1..];
        return new(clean[..cut].Trim(), body);
    }

    /// <summary>Los apartados van separados por una línea en blanco (lote 39d).</summary>
    public static IReadOnlyList<UnsafeHelpSection> Sections(string? help) =>
        BlankLine().Split(help ?? string.Empty)
            .Where(static part => !string.IsNullOrWhiteSpace(part))
            .Select(SplitSection)
            .ToArray();

    /// <summary>Lo decide la organización: la directiva fija el caso o lo
    /// prohíbe en un bloqueo. Un directorio de ajustes no disponible no.</summary>
    public static bool OrganizationDecides(UnsafeExceptionState item)
    {
        ArgumentNullException.ThrowIfNull(item);
        return item.Fixed || (!item.Activable && !item.ConfigUnavailable &&
            !string.IsNullOrWhiteSpace(item.OrganizationKey));
    }

    public static IReadOnlyList<UnsafeHelpSection> HelpSections(string? help,
        string? organizationSection, bool decidedByOrganization)
    {
        var all = Sections(help);
        if (!decidedByOrganization) return all;
        var kept = all.Take(SectionsKeptForOrganization).ToList();
        if (!string.IsNullOrWhiteSpace(organizationSection)) kept.Add(SplitSection(organizationSection));
        return kept;
    }

    /// <summary>Nota de una fila de Configuración que no se puede cambiar. Cada
    /// cosa se dice una vez: el estado ya dice «Desactivado por la
    /// organización» o que la directiva no se puede leer.</summary>
    public static string PolicyNoteKey(UnsafeExceptionState item)
    {
        ArgumentNullException.ThrowIfNull(item);
        if (item.PolicyUnreadable) return string.Empty;
        if (item.Fixed)
            return item.Enabled ? "excepciones.pantalla.solo_lectura" : item.OrganizationKey;
        return !item.Activable && item.ConfigUnavailable ? "excepciones.directorio_no_disponible" : string.Empty;
    }

    /// <summary>Fecha y hora locales con el formato del idioma de la
    /// interfaz («format.datetime_minutes», disposición de Go): el año siempre
    /// con cuatro cifras y el mismo resultado que Qt.</summary>
    public static string DateText(DateTimeOffset instant, string? layout, TimeZoneInfo zone)
    {
        ArgumentNullException.ThrowIfNull(zone);
        var local = TimeZoneInfo.ConvertTime(instant, zone);
        var pattern = layout ?? string.Empty;
        if (!pattern.Contains("2006", StringComparison.Ordinal) || !pattern.Contains("01", StringComparison.Ordinal) ||
            !pattern.Contains("02", StringComparison.Ordinal))
            pattern = DefaultDateLayout;
        return LayoutToken().Replace(pattern, match => match.Value switch
        {
            "2006" => local.Year.ToString("0000", CultureInfo.InvariantCulture),
            "01" => local.Month.ToString("00", CultureInfo.InvariantCulture),
            "02" => local.Day.ToString("00", CultureInfo.InvariantCulture),
            "15" => local.Hour.ToString("00", CultureInfo.InvariantCulture),
            "04" => local.Minute.ToString("00", CultureInfo.InvariantCulture),
            _ => local.Second.ToString("00", CultureInfo.InvariantCulture),
        });
    }
}
