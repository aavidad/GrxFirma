// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class OutputOpeningPolicyTests
{
    [TestMethod]
    [DataRow(@"C:\Datos\nomina_desprotegido.exe")]
    [DataRow(@"C:\Datos\a.hta")]
    [DataRow(@"C:\Datos\a.lnk")]
    [DataRow(@"C:\Datos\a.js")]
    [DataRow(@"C:\Datos\a.scr")]
    [DataRow(@"C:\Datos\a.bat")]
    [DataRow(@"C:\Datos\a.cmd")]
    [DataRow(@"C:\Datos\a.msi")]
    [DataRow(@"C:\Datos\a.url")]
    [DataRow(@"C:\Datos\a.cpl")]
    [DataRow(@"C:\Datos\a.txt:evil")]
    [DataRow(@"C:\Datos\sin_extension")]
    [DataRow(@"C:\Datos\a.bin")]
    [DataRow("")]
    public void RejectsExecutableAndUnknownTypes(string path)
    {
        Assert.IsFalse(OutputOpeningPolicy.IsDocumentToOpen(path));
    }

    [TestMethod]
    public void RejectsMissingPath()
    {
        Assert.IsFalse(OutputOpeningPolicy.IsDocumentToOpen(null));
    }

    [TestMethod]
    [DataRow(@"C:\Datos\a.pdf")]
    [DataRow(@"C:\Datos\a.PDF")]
    [DataRow(@"C:\Datos\a.docx")]
    [DataRow(@"C:\Datos\a.xml")]
    [DataRow(@"C:\Datos\firma.xsig")]
    public void AcceptsDocumentTypes(string path)
    {
        Assert.IsTrue(OutputOpeningPolicy.IsDocumentToOpen(path));
    }
}
