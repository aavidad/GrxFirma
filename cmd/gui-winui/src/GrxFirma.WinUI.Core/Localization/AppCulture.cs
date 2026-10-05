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
    private static readonly IReadOnlyDictionary<string, string> Tags =
        new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            ["es"] = "es-ES",
            ["en"] = "en-GB",
            ["ca"] = "ca-ES",
            ["va"] = "ca-ES-valencia",
            ["gl"] = "gl-ES",
            ["eu"] = "eu-ES",
            ["fr"] = "fr-FR",
            ["de"] = "de-DE",
            ["it"] = "it-IT",
            ["pt"] = "pt-PT",
            ["zh"] = "zh-CN",
        };

    public static string Tag(string? language) =>
        language is not null && Tags.TryGetValue(language, out var tag) ? tag : "es-ES";

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
