// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DeferredTreePassTests
{
    [TestMethod]
    public void OpenWindow_RunsThePass()
    {
        var gate = new DeferredTreePass();
        var runs = 0;
        Assert.IsTrue(gate.TryRun(() => runs++));
        Assert.AreEqual(1, runs);
        Assert.IsFalse(gate.IsClosed);
    }

    // Regresión 0.0.118: la traducción encolada tras mover el sello se
    // ejecutaba con la ventana ya cerrada y tumbaba el editor del portal.
    [TestMethod]
    public void ClosedWindow_SkipsQueuedPass()
    {
        var gate = new DeferredTreePass();
        gate.Close();
        var runs = 0;
        Assert.IsFalse(gate.TryRun(() => runs++));
        Assert.AreEqual(0, runs);
        Assert.IsTrue(gate.IsClosed);
    }

    [TestMethod]
    public void TreeAlreadyTornDown_DiscardsPassWithoutThrowing()
    {
        var gate = new DeferredTreePass();
        const int unexpected = unchecked((int)0x8000FFFF);
        Assert.IsFalse(gate.TryRun(() => throw new COMException(null, unexpected)));
        // Una pasada fallida no impide las siguientes mientras la ventana vive.
        Assert.IsTrue(gate.TryRun(() => { }));
    }

    [TestMethod]
    public void OtherFailures_StillSurface()
    {
        var gate = new DeferredTreePass();
        Assert.ThrowsExactly<InvalidOperationException>(
            () => { gate.TryRun(() => throw new InvalidOperationException()); });
    }
}
