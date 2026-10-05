// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;

namespace GrxFirma.WinUI.Core.Localization;

// Cultura de formato del idioma elegido en la aplicación, independiente del
// idioma de Windows: fechas, importes y controles propios de WinUI (selector
// de fecha y hora) se muestran en el mismo idioma que el resto de la pantalla.
public static class AppCulture
{
    // Etiquetas BCP 47 en minúsculas: no distinguen mayúsculas.
    private static readonly IReadOnlyDictionary<string, string> Tags =
        new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            ["es"] = "es-es",
            ["en"] = "en-gb",
            ["ca"] = "ca-es",
            ["va"] = "ca-es-valencia",
            ["gl"] = "gl-es",
            ["eu"] = "eu-es",
            ["fr"] = "fr-fr",
            ["de"] = "de-de",
            ["it"] = "it-it",
            ["pt"] = "pt-pt",
            ["zh"] = "zh-cn",
        };

    public static string Tag(string? language) =>
        language is not null && Tags.TryGetValue(language, out var tag) ? tag : "es-es";

    public static CultureInfo For(string? language)
    {
        var tag = Tag(language);
        try
        {
            return CultureInfo.GetCultureInfo(tag);
        }
        catch (CultureNotFoundException)
        {
            // Sin datos ICU para la variante (p. ej. valenciano), la base.
            try
            {
                return CultureInfo.GetCultureInfo(tag.Split('-')[0]);
            }
            catch (CultureNotFoundException)
            {
                return CultureInfo.InvariantCulture;
            }
        }
    }
}
