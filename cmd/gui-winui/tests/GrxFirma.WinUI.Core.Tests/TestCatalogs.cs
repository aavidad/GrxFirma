// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Tests;

/// <summary>Catálogos reales del repositorio para las pruebas de textos.</summary>
internal static class TestCatalogs
{
    internal static readonly string[] Languages =
        ["ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"];

    private static readonly Lazy<CatalogLocalizer> Localizer =
        new(() => new CatalogLocalizer(FindLocales(), "es"));

    internal static string Es(string key) => Localizer.Value.Text("es", key);

    internal static Func<string, string> In(string language) =>
        key => Localizer.Value.Text(language, key);

    private static string FindLocales()
    {
        for (var directory = new DirectoryInfo(AppContext.BaseDirectory); directory is not null; directory = directory.Parent)
        {
            var candidate = Path.Combine(directory.FullName, "internal", "adapters", "outbound", "common", "localizador", "locales");
            if (File.Exists(Path.Combine(candidate, "es.json")))
            {
                return candidate;
            }
        }

        Assert.Fail("No se encuentran los catálogos del repositorio.");
        return string.Empty;
    }
}
