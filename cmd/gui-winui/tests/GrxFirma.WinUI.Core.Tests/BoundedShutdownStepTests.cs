// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Diagnostics;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class BoundedShutdownStepTests
{
    [TestMethod]
    public void CompletedStep_ReturnsTrue()
    {
        var closed = false;
        Assert.IsTrue(BoundedShutdownStep.Run(async () =>
        {
            await Task.Yield();
            closed = true;
        }, TimeSpan.FromSeconds(5)));
        Assert.IsTrue(closed);
    }

    // Un motor que no responde no debe dejar la aplicación colgada al salir.
    [TestMethod]
    public void StuckStep_ReturnsFalseWhenTheTimeoutExpires()
    {
        var never = new TaskCompletionSource();
        var clock = Stopwatch.StartNew();
        Assert.IsFalse(BoundedShutdownStep.Run(() => never.Task,
            TimeSpan.FromMilliseconds(100)));
        Assert.IsTrue(clock.Elapsed < TimeSpan.FromSeconds(5));
    }

    // Un fallo al cerrar el canal no impide terminar la salida.
    [TestMethod]
    public void FailingStep_CountsAsFinishedWithoutThrowing()
    {
        Assert.IsTrue(BoundedShutdownStep.Run(
            () => Task.FromException(new IOException("canal roto")),
            TimeSpan.FromSeconds(5)));
        Assert.IsTrue(BoundedShutdownStep.Run(
            () => throw new InvalidOperationException("fallo síncrono"),
            TimeSpan.FromSeconds(5)));
    }

    // Se llama desde el hilo de la interfaz, que queda bloqueado esperando:
    // el paso no puede depender de volver a ese hilo.
    [TestMethod]
    public void Step_DoesNotResumeOnTheCallersContext()
    {
        var previous = SynchronizationContext.Current;
        var blocked = new BlockedUiContext();
        SynchronizationContext.SetSynchronizationContext(blocked);
        try
        {
            Assert.IsTrue(BoundedShutdownStep.Run(async () =>
            {
                await Task.Delay(10);
                await Task.Yield();
            }, TimeSpan.FromSeconds(5)));
            Assert.AreEqual(0, blocked.Posts);
        }
        finally
        {
            SynchronizationContext.SetSynchronizationContext(previous);
        }
    }

    [TestMethod]
    public void InvalidArguments_AreRejected()
    {
        Assert.ThrowsExactly<ArgumentNullException>(
            () => BoundedShutdownStep.Run(null!, TimeSpan.FromSeconds(1)));
        Assert.ThrowsExactly<ArgumentOutOfRangeException>(
            () => BoundedShutdownStep.Run(() => Task.CompletedTask, TimeSpan.Zero));
    }

    private sealed class BlockedUiContext : SynchronizationContext
    {
        private int _posts;

        public int Posts => Volatile.Read(ref _posts);

        public override void Post(SendOrPostCallback d, object? state) =>
            Interlocked.Increment(ref _posts);

        public override void Send(SendOrPostCallback d, object? state) =>
            Interlocked.Increment(ref _posts);
    }
}
