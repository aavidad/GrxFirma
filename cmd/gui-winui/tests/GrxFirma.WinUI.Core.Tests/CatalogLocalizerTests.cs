// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class CatalogLocalizerTests
{
    [TestMethod]
    public void SelectedLanguageAndSpanishValueUseTheSharedCatalog()
    {
        var directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(directory);
        try
        {
            File.WriteAllText(Path.Combine(directory, "es.json"),
                """{"Cambios sin guardar":"Cambios sin guardar","semantic.key":"Firmar"}""");
            File.WriteAllText(Path.Combine(directory, "en.json"),
                """{"Cambios sin guardar":"Unsaved changes","semantic.key":"Sign"}""");
            var localizer = new CatalogLocalizer(directory, "es-ES");
            Assert.AreEqual("es", localizer.Language);
            Assert.AreEqual("va", CatalogLocalizer.Normalize("va-ES"));
            Assert.AreEqual("zh", CatalogLocalizer.Normalize("zh-Hans"));
            Assert.AreEqual("es", CatalogLocalizer.Normalize("../private"));
            Assert.IsTrue(localizer.SetLanguage("en-GB"));
            Assert.AreEqual("Unsaved changes", localizer.Text("Cambios sin guardar"));
            Assert.AreEqual("Sign", localizer.TranslateVisibleText("Firmar"));
            Assert.IsFalse(localizer.SetLanguage("en"));
            Assert.AreEqual("Firmar", localizer.Text("es", "semantic.key"));
            Assert.AreEqual("Firmar", localizer.Text("fr", "semantic.key"));
            Assert.AreEqual("Texto nuevo", localizer.TranslateVisibleText("Texto nuevo"));
            Assert.AreEqual("es", CatalogLocalizer.Normalize("unknown"));
        }
        finally
        {
            Directory.Delete(directory, recursive: true);
        }
    }

    [TestMethod]
    public void EmptyCatalogDirectoryIsRejected()
    {
        Assert.ThrowsExactly<ArgumentException>(() => new CatalogLocalizer(""));
    }

    [TestMethod]
    public void VisibleTemplateKeepsFileNameWhenLanguageChanges()
    {
        var directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(directory);
        try
        {
            File.WriteAllText(Path.Combine(directory, "es.json"),
                """{"La firma se guardó como {0}, pero no pudo validarse.":"La firma se guardó como {0}, pero no pudo validarse.","Faltan los datos del {0}.":"Faltan los datos del {0}.","El NIF del {0}":"El NIF del {0}","{0} es obligatorio.":"{0} es obligatorio.","emisor":"emisor","{0}: {1}":"{0}: {1}"}""");
            File.WriteAllText(Path.Combine(directory, "en.json"),
                """{"La firma se guardó como {0}, pero no pudo validarse.":"The signature was saved as {0}, but could not be validated.","Faltan los datos del {0}.":"Details for the {0} are missing.","El NIF del {0}":"The {0}'s tax ID","{0} es obligatorio.":"{0} is required.","emisor":"issuer","{0}: {1}":"{0} - {1}"}""");
            var localizer = new CatalogLocalizer(directory, "en");
            var source = "La firma se guardó como acta.pdf, pero no pudo validarse.";
            Assert.AreEqual(
                "The signature was saved as acta.pdf, but could not be validated.",
                localizer.TranslateVisibleText(source));
            Assert.AreEqual("Header: value",
                localizer.TranslateVisibleText("Header: value"));
            Assert.AreEqual("Details for the emisor are missing.",
                localizer.TranslateVisibleText("Faltan los datos del emisor."));
            Assert.AreEqual("Details for the issuer are missing.",
                localizer.TranslateVisibleText(
                    "Faltan los datos del emisor.", translateCapturedSpanish: true));
            Assert.AreEqual("The issuer's tax ID is required.",
                localizer.TranslateVisibleText(
                    "El NIF del emisor es obligatorio.", translateCapturedSpanish: true));
            Assert.IsTrue(localizer.SetLanguage("es"));
            Assert.AreEqual(source, localizer.TranslateVisibleText(source));
            Assert.IsTrue(localizer.SetLanguage("en"));
            Assert.AreEqual(
                "The signature was saved as acta.pdf, but could not be validated.",
                localizer.TranslateVisibleText(source));
        }
        finally
        {
            Directory.Delete(directory, recursive: true);
        }
    }
}
