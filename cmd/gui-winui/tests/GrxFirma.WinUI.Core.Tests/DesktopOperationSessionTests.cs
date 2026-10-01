// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopOperationSessionTests
{
    [TestMethod]
    public void Attach_PublishesOnlyAdvertisedActions()
    {
        var transport = new FakeIpcClient("hello", "sign");
        var session = new DesktopOperationSession();
        var notifications = 0;
        session.AvailabilityChanged += (_, _) => notifications++;

        session.Attach(transport);

        Assert.IsTrue(session.IsConnected);
        Assert.IsTrue(session.Supports("sign"));
        Assert.IsFalse(session.Supports("verify"));
        Assert.IsTrue(session.TryGetOperations(
            "sign",
            out var operations));
        Assert.IsNotNull(operations);
        Assert.IsFalse(session.TryGetOperations(
            "verify",
            out _));
        Assert.AreEqual(1, notifications);
    }

    [TestMethod]
    public void Detach_RequiresThePublishedTransport()
    {
        var published = new FakeIpcClient("hello", "verify");
        var unrelated = new FakeIpcClient("hello", "verify");
        var session = new DesktopOperationSession();
        var notifications = 0;
        session.AvailabilityChanged += (_, _) => notifications++;
        session.Attach(published);

        session.Detach(unrelated);
        Assert.IsTrue(session.IsConnected);
        Assert.AreEqual(1, notifications);

        session.Detach(published);
        Assert.IsFalse(session.IsConnected);
        Assert.IsFalse(session.Supports("verify"));
        Assert.AreEqual(2, notifications);
    }

    [TestMethod]
    public void Attach_RejectsTransportWithoutNegotiatedHello()
    {
        var session = new DesktopOperationSession();

        Assert.ThrowsExactly<InvalidOperationException>(
            () => session.Attach(new FakeIpcClient()));
        Assert.IsFalse(session.IsConnected);
    }

    [TestMethod]
    public void TemporaryCredentialTracking_SurvivesPageNavigation()
    {
        var session = new DesktopOperationSession();
        session.Attach(new FakeIpcClient(
            "hello",
            "certificates",
            "remove_temporary_certificate",
            "clear_temporary_certificates"));

        // La primera página registra la identidad confirmada por el motor.
        session.TrackTemporaryCertificate("temporary-1");

        // Una página nueva consulta el mismo estado de sesión.
        Assert.IsTrue(
            session.IsTemporaryCertificateTracked("temporary-1"));
        Assert.IsTrue(session.Supports(
            "remove_temporary_certificate"));
        Assert.IsTrue(session.Supports(
            "clear_temporary_certificates"));

        Assert.IsTrue(
            session.UntrackTemporaryCertificate("temporary-1"));
        Assert.IsFalse(
            session.IsTemporaryCertificateTracked("temporary-1"));
    }

    [TestMethod]
    public void TemporaryCredentialTracking_ClearAndDetachCannotLeakState()
    {
        var transport = new FakeIpcClient(
            "hello",
            "certificates");
        var session = new DesktopOperationSession();
        session.Attach(transport);
        session.TrackTemporaryCertificate("temporary-1");
        session.TrackTemporaryCertificate("temporary-2");

        session.ClearTrackedTemporaryCertificates();

        Assert.IsFalse(
            session.IsTemporaryCertificateTracked("temporary-1"));
        Assert.IsFalse(
            session.IsTemporaryCertificateTracked("temporary-2"));

        session.TrackTemporaryCertificate("temporary-3");
        session.Detach(transport);

        Assert.IsFalse(
            session.IsTemporaryCertificateTracked("temporary-3"));
    }

    [TestMethod]
    public void TemporaryCredentialTracking_RejectsUnusableIdentifiers()
    {
        var session = new DesktopOperationSession();

        Assert.ThrowsExactly<ArgumentException>(
            () => session.TrackTemporaryCertificate(" "));
        Assert.ThrowsExactly<ArgumentException>(
            () => session.TrackTemporaryCertificate(
                new string('x', 1025)));
        Assert.IsFalse(
            session.IsTemporaryCertificateTracked(string.Empty));
        Assert.IsFalse(
            session.UntrackTemporaryCertificate(string.Empty));
    }

    private sealed class FakeIpcClient(params string[] actions) : IIpcClient
    {
        public IpcHello? ServerHello { get; } = actions.Length == 0
            ? null
            : new IpcHello
            {
                Protocol = DesktopIpcProtocol.Name,
                Version = DesktopIpcProtocol.Version,
                Actions = actions,
            };

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default) =>
            throw new NotSupportedException();

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }
}
