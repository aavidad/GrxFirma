// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class SafeIpcTextTests
{
    [TestMethod]
    public void Clean_RemovesControlsAndUnicodeFormatCharacters()
    {
        var cleaned = SafeIpcText.Clean(
            "  Inicio\u0000\u202eengaño\u2066 fin\n  ",
            100,
            "fallback");

        Assert.AreEqual("Inicioengaño fin", cleaned);
        Assert.IsFalse(cleaned.Any(char.IsControl));
    }

    [TestMethod]
    public void Clean_DoesNotSplitUtf16SurrogatePairs()
    {
        Assert.AreEqual(
            "fallback",
            SafeIpcText.Clean("😀", 1, "fallback"));
        Assert.AreEqual(
            "😀",
            SafeIpcText.Clean("😀texto", 2, "fallback"));
    }

    [TestMethod]
    public void Clean_PreservesUsefulLineBreaksInsideTheMessage()
    {
        Assert.AreEqual(
            "primera\nsegunda",
            SafeIpcText.Clean(
                "primera\nsegunda",
                100,
                "fallback"));
    }
}
