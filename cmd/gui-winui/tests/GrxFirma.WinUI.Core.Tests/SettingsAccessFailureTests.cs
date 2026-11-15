// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Tests;

// Lote 39g: ajustes sin permiso o de otra cuenta. WinUI pinta el texto de su
// catálogo y separa la orden de la última línea para poder copiarla.
[TestClass]
public sealed class SettingsAccessFailureTests
{
    private const string Icacls = "icacls \"%APPDATA%\\GrxFirma\" /grant \"%USERNAME%:(OI)(CI)F\" /T";

    [TestMethod]
    public void OnlyClosedCodesAreExplained_AndCommandIsSplitFromLastLine()
    {
        var catalog = new Dictionary<string, string>
        {
            ["error.settings_sin_permiso"] = "No se pueden abrir los ajustes. Ejecute:\n" + Icacls,
            ["error.settings_otro_usuario"] = "La carpeta es de otra cuenta.",
        };
        string Text(string key) => catalog.TryGetValue(key, out var value) ? value : key;

        var noPermission = SettingsAccessFailure.From("settings_no_permission", Text);
        Assert.IsNotNull(noPermission);
        Assert.AreEqual("No se pueden abrir los ajustes. Ejecute:", noPermission.Explanation);
        Assert.AreEqual(Icacls, noPermission.Command);

        var otherUser = SettingsAccessFailure.From("settings_other_user", Text);
        Assert.IsNotNull(otherUser);
        Assert.AreEqual("La carpeta es de otra cuenta.", otherUser.Explanation);
        Assert.AreEqual(string.Empty, otherUser.Command);

        Assert.IsNull(SettingsAccessFailure.From("operation_failed", Text));
        Assert.IsNull(SettingsAccessFailure.From(null, Text));
        Assert.IsNull(SettingsAccessFailure.From("error.settings_sin_permiso", Text));
        // Sin la clave en el catálogo queda el camino de siempre.
        Assert.IsNull(SettingsAccessFailure.From("settings_no_permission", key => key));
    }

    [TestMethod]
    public void RealCatalogsCarryTheLiteralCommandInEveryLanguage()
    {
        var locales = FindLocales();
        foreach (var language in new[] { "ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh" })
        {
            var localizer = new CatalogLocalizer(locales, language);
            var failure = SettingsAccessFailure.From(
                SettingsAccessFailure.NoPermissionCode,
                key => localizer.Text(language, key));
            Assert.IsNotNull(failure, language);
            Assert.AreEqual(Icacls, failure.Command, language);
            Assert.IsFalse(failure.Explanation.Contains('\n'), language);
            Assert.IsFalse(string.IsNullOrWhiteSpace(failure.Explanation), language);
        }
    }

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
