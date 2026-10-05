// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DistinguishedNameTextTests
{
    [TestMethod]
    [DataRow("SERIALNUMBER=IDCES-99999999R,2.5.4.4=#1311,CN=EIDAS CERTIFICADO PRUEBAS - 99999999R,C=ES", "EIDAS CERTIFICADO PRUEBAS - 99999999R")]
    [DataRow("CN=Pérez\\, Ana,O=Diputación", "Pérez, Ana")]
    [DataRow("cn = Ana ", "Ana")]
    [DataRow("O=Sin nombre común", "O=Sin nombre común")]
    [DataRow("Ana Pérez", "Ana Pérez")]
    [DataRow("", "")]
    public void ExtractsTheCommonName(string input, string expected) =>
        Assert.AreEqual(expected, DistinguishedNameText.CommonName(input));
}
