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
}
