// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;
using GrxFirma.WinUI.Core.Layout;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class OptionalSystemSignalTests
{
    // Windows 10 22H2: add_HighContrastChanged lanza COMException 0x80070490
    // (ERROR_NOT_FOUND) y la ventana se cerraba antes de mostrarse.
    [TestMethod]
    public void MissingSignal_OnWindows10_DoesNotCloseTheApp()
    {
        var subscribed = OptionalSystemSignal.TrySubscribe(
            () => throw new COMException("Elemento no encontrado.", unchecked((int)0x80070490)));
        Assert.IsFalse(subscribed);
    }

    [TestMethod]
    public void AnyPlatformFailure_IsAbsorbed_ButOutOfMemoryIsNot()
    {
        Assert.IsFalse(OptionalSystemSignal.TrySubscribe(() => throw new InvalidOperationException()));
        Assert.IsFalse(OptionalSystemSignal.TrySubscribe(() => throw new UnauthorizedAccessException()));
        Assert.ThrowsExactly<OutOfMemoryException>(
            () => OptionalSystemSignal.TrySubscribe(() => throw new OutOfMemoryException()));
    }

    [TestMethod]
    public void AvailableSignal_IsSubscribed()
    {
        var calls = 0;
        Assert.IsTrue(OptionalSystemSignal.TrySubscribe(() => calls++));
        Assert.AreEqual(1, calls);
    }
}
