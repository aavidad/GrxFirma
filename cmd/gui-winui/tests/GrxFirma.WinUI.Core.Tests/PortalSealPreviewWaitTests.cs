// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Diagnostics;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class PortalSealPreviewWaitTests
{
    private static readonly TimeSpan ShortRetry = TimeSpan.FromMilliseconds(20);

    private static Task<bool> Wait(
        Func<bool> ready,
        Func<bool> needsRefresh,
        Func<Task> refresh,
        TimeSpan timeout,
        int attempts = 5,
        CancellationToken cancellationToken = default) =>
        PortalSealPreviewWait.WaitAsync(ready, needsRefresh, refresh, timeout,
            ShortRetry, attempts, TimeProvider.System, cancellationToken);

    [TestMethod]
    public async Task ReadyAfterFirstRefresh_EnablesPlacementWithoutRetrying()
    {
        var loaded = false;
        var calls = 0;
        var ready = await Wait(() => loaded, () => !loaded,
            () => { calls++; loaded = true; return Task.CompletedTask; },
            TimeSpan.FromSeconds(5));
        Assert.IsTrue(ready);
        Assert.AreEqual(1, calls);
    }

    // Regresión 0.0.117: la vista previa no terminaba y el botón «Firmar con
    // el sello aquí» seguía desactivado sin aviso ni salida.
    [TestMethod]
    public async Task RefreshThatNeverEnds_StopsWaitingAtTheDeadline()
    {
        var never = new TaskCompletionSource();
        var watch = Stopwatch.StartNew();
        var ready = await Wait(() => false, () => true, () => never.Task,
            TimeSpan.FromMilliseconds(300));
        watch.Stop();
        Assert.IsFalse(ready);
        Assert.IsTrue(watch.ElapsedMilliseconds < 5000);
        Assert.IsFalse(never.Task.IsCompleted);
    }

    [TestMethod]
    public async Task RepeatedFailures_StopAfterTheAttemptLimit()
    {
        var calls = 0;
        var ready = await Wait(() => false, () => true,
            () => { calls++; return Task.CompletedTask; },
            TimeSpan.FromSeconds(30), attempts: 3);
        Assert.IsFalse(ready);
        Assert.AreEqual(3, calls);
    }

    [TestMethod]
    public async Task PageAlreadyLoading_WaitsForRenderingWithoutReloading()
    {
        var watch = Stopwatch.StartNew();
        var calls = 0;
        var ready = await Wait(() => watch.ElapsedMilliseconds > 100, () => false,
            () => { calls++; return Task.CompletedTask; },
            TimeSpan.FromSeconds(5));
        Assert.IsTrue(ready);
        Assert.AreEqual(0, calls);
    }

    [TestMethod]
    public async Task RenderingThatNeverConfirms_EndsAtTheDeadline()
    {
        var ready = await Wait(() => false, () => false, () => Task.CompletedTask,
            TimeSpan.FromMilliseconds(200));
        Assert.IsFalse(ready);
    }

    [TestMethod]
    public async Task ClosedWindow_CancelsTheWait()
    {
        using var cancellation = new CancellationTokenSource();
        cancellation.Cancel();
        await Assert.ThrowsExactlyAsync<OperationCanceledException>(() =>
            Wait(() => false, () => true, () => Task.CompletedTask,
                TimeSpan.FromSeconds(5), cancellationToken: cancellation.Token));
    }

    [TestMethod]
    public async Task InvalidLimits_AreRejected()
    {
        await Assert.ThrowsExactlyAsync<ArgumentOutOfRangeException>(() =>
            Wait(() => true, () => false, () => Task.CompletedTask, TimeSpan.Zero));
        await Assert.ThrowsExactlyAsync<ArgumentOutOfRangeException>(() =>
            Wait(() => true, () => false, () => Task.CompletedTask,
                TimeSpan.FromSeconds(1), attempts: 0));
    }
}
