// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class EniValidationTests
{
    [TestMethod]
    public void MetadataRejectsInvalidCharactersAndLengths()
    {
        Assert.IsNull(EniValidation.OrganError("L01180877, A00000000"));
        Assert.IsNotNull(EniValidation.OrganError("LLLLLLLLL"));
        Assert.IsNotNull(EniValidation.OrganError("L01180877\n"));
        Assert.IsNull(EniValidation.IdentifierError("ES_L01180877_2026_" + new string('x', 30)));
        Assert.IsNotNull(EniValidation.IdentifierError("ES_L01180877_2026_" + new string('x', 31)));
        Assert.IsNotNull(EniValidation.IdentifierError("", required: true));
        Assert.IsNull(EniValidation.IdentifierError(""));
        Assert.IsNull(EniValidation.ClassificationError("L01180877_PRO_LICENCIAS"));
        Assert.IsNull(EniValidation.ClassificationError("123456"));
        Assert.IsNotNull(EniValidation.ClassificationError("<xml>"));
        Assert.IsNotNull(EniValidation.InterestedError("x\0"));
        Assert.IsNotNull(EniValidation.FormatError("<PDF>"));
    }

    [TestMethod]
    public void DatesPreserveDayTimeAndIncludeZone()
    {
        var date = new DateTimeOffset(2026, 10, 4, 0, 0, 0, TimeSpan.Zero);
        var text = EniValidation.SerializeDate(date, new TimeSpan(13, 42, 0));
        var parsed = DateTimeOffset.ParseExact(text, "yyyy-MM-ddTHH:mm:sszzz", CultureInfo.InvariantCulture);
        Assert.AreEqual(4, parsed.Day);
        Assert.AreEqual(13, parsed.Hour);
        Assert.AreEqual(42, parsed.Minute);
        Assert.ThrowsExactly<ArgumentException>(() => EniValidation.SerializeDate(null, TimeSpan.Zero));
        Assert.ThrowsExactly<ArgumentException>(() => EniValidation.SerializeDate(date, TimeSpan.FromHours(24)));
    }
}
