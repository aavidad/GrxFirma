// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VeriFactuQrDisplayTests
{
    [TestMethod]
    public void SpanishUsesCommaAndSlashes()
    {
        var culture = AppCulture.For("es");
        Assert.AreEqual("01/09/2024", VeriFactuQrDisplay.Date("01-09-2024", culture));
        StringAssert.StartsWith(VeriFactuQrDisplay.Amount("241.4", culture).Replace(' ', ' '), "241,40");
        StringAssert.Contains(VeriFactuQrDisplay.Amount("241.4", culture), "€");
    }

    [TestMethod]
    public void EnglishUsesPointDecimal()
    {
        var culture = AppCulture.For("en");
        StringAssert.Contains(VeriFactuQrDisplay.Amount("241.4", culture), "241.40");
        Assert.AreEqual("01/09/2024", VeriFactuQrDisplay.Date("01-09-2024", culture));
    }

    [TestMethod]
    [DataRow("2024-09-01")]
    [DataRow("31-02-2024")]
    [DataRow("")]
    public void UnexpectedDatesAreShownAsGiven(string raw) =>
        Assert.AreEqual(raw, VeriFactuQrDisplay.Date(raw, AppCulture.For("es")));

    [TestMethod]
    [DataRow("7,2")]
    [DataRow("1e3")]
    [DataRow("abc")]
    public void UnexpectedAmountsAreShownAsGiven(string raw) =>
        Assert.AreEqual(raw, VeriFactuQrDisplay.Amount(raw, AppCulture.For("es")));

    [TestMethod]
    public void EveryAppLanguageHasACulture()
    {
        foreach (var language in new[] { "es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh" })
            Assert.AreNotEqual(string.Empty, AppCulture.For(language).Name, language);
    }
}
