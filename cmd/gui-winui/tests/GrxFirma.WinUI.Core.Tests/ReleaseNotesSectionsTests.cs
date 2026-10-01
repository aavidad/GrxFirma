// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class ReleaseNotesSectionsTests
{
    [TestMethod]
    public void UpdateFrom99To101ShowsOnly100And101()
    {
        const string source = "<!-- licencia -->\n## 0.0.101 — 2026-10-01\n- C\n" +
            "## Próxima versión (sin publicar)\n- futuro\n" +
            "## 0.0.99 — 2026-09-30\n- A\n" +
            "## 0.0.100 — 2026-10-01\n- B\n";
        var actual = ReleaseNotesSections.Select(source, "0.0.101", "0.0.99");
        Assert.IsTrue(actual.Contains("## 0.0.101"));
        Assert.IsTrue(actual.Contains("## 0.0.100"));
        Assert.IsFalse(actual.Contains("## 0.0.99"));
        Assert.IsFalse(actual.Contains("licencia"));
        Assert.IsFalse(actual.Contains("futuro"));
        Assert.IsTrue(actual.IndexOf("0.0.101", StringComparison.Ordinal) <
            actual.IndexOf("0.0.100", StringComparison.Ordinal));
    }

    [TestMethod]
    public void StripsActiveMarkupAndLinks()
    {
        const string source = "## 0.0.101 — 2026-10-01\n" +
            "- [Ayuda](javascript:alert(1)) <script>código</script>\n";
        var actual = ReleaseNotesSections.Select(source, "0.0.101");
        Assert.IsTrue(actual.Contains("Ayuda"));
        Assert.IsFalse(actual.Contains("javascript:"));
        Assert.IsFalse(actual.Contains('<'));
    }
}
