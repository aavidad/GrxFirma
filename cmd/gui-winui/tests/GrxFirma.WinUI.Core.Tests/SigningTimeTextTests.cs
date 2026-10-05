// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Localization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class SigningTimeTextTests
{
    [TestMethod]
    public void SpanishLocalTimeWithSeconds()
    {
        Assert.IsTrue(SigningTimeText.TryFormat("2026-10-05T11:00:44Z", "signed_attribute",
            AppCulture.For("es"), TimeZoneInfo.Utc, out var text));
        Assert.AreEqual("05/10/2026 11:00:44", text);
    }

    [TestMethod]
    [DataRow("", "timestamp")]
    [DataRow("05/10/2026", "timestamp")]
    [DataRow("2026-10-05T11:00:44Z", "")]
    [DataRow("2026-10-05T11:00:44Z", "pdf_m")]
    public void UnknownValuesAreNotShown(string raw, string source)
    {
        Assert.IsFalse(SigningTimeText.TryFormat(raw, source, AppCulture.For("es"), TimeZoneInfo.Utc, out var text));
        Assert.AreEqual(string.Empty, text);
    }

    [TestMethod]
    public void SignerSummaryReadsSigningTimeFromIpc()
    {
        var summary = JsonSerializer.Deserialize<VerifySignerSummary>(
            """{"subject":"CN=Ana","signingTime":"2026-10-05T11:00:44Z","signingTimeSource":"timestamp"}""");
        Assert.IsNotNull(summary);
        Assert.AreEqual("2026-10-05T11:00:44Z", summary.SigningTime);
        Assert.AreEqual(SigningTimeText.FromTimestamp, summary.SigningTimeSource);
    }
}
