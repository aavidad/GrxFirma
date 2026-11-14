// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Localization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class ContextualFailureAssistantTests
{
    [TestMethod]
    public void RemoteCodeAndProbableOwnerDoNotInventNetworkOrServerEvidence()
    {
        var diagnostic = new OperationDiagnostic
        {
            FailureCode = "SAF_26",
            Category = "remote_service",
            LikelyOwner = "government_afirma",
            RequestId = "request-27",
            Action = "sign_batch",
            Steps = [new() { Code = "SAF_26", EvidenceRef = "phase:operation",
                Status = DiagnosticStepStatus.Failure, Owner = "remote_service" }],
        };
        var rows = ContextualFailureAssistant.Rows(diagnostic);
        Assert.AreEqual(6, rows.Count);
        Assert.IsTrue(rows.All(row => row.Detail == CatalogLocalizer.Shared.Text("report.status.unknown")));
        var display = ContextualFailureAssistant.ForDisplay(diagnostic);
        Assert.AreEqual("unknown", display.LikelyOwner);
        Assert.AreEqual("unknown", display.Steps.Single().Owner);
        Assert.AreEqual("request-27", display.RequestId);
        Assert.AreEqual("sign_batch", display.Action);
        Assert.AreEqual("government_afirma", diagnostic.LikelyOwner);
        Assert.AreEqual("winui.l27.assistant.connection", ContextualFailureAssistant.ActionKey(diagnostic));
    }

    [TestMethod]
    public void LocalReadingDoesNotProveRemoteClockSynchronization()
    {
        var local = new LocalFailureEvidence(new() { OkCount = 2, FailCount = 1 }, Clock());
        var rows = ContextualFailureAssistant.Rows(new(), local);
        Assert.AreNotEqual(CatalogLocalizer.Shared.Text("report.status.unknown"), rows[0].Detail);
        Assert.AreNotEqual(CatalogLocalizer.Shared.Text("report.status.unknown"), rows[1].Detail);
        Assert.IsTrue(rows.Skip(2).All(row => row.Detail == CatalogLocalizer.Shared.Text("report.status.unknown")));
    }

    [TestMethod]
    public void IncoherentOrUnreferencedEvidenceStaysUnknown()
    {
        var rows = ContextualFailureAssistant.Rows(new()
        {
            Steps = [new() { Code = "government_afirma", Status = DiagnosticStepStatus.Success }],
        }, new(new() { OkCount = -1 }, new() { ThresholdSeconds = 5 }));
        Assert.IsTrue(rows.All(row => row.Detail == CatalogLocalizer.Shared.Text("report.status.unknown")));
    }

    [TestMethod]
    public async Task LocalChecksOnlyUseExistingParameterlessReadContracts()
    {
        var transport = new LocalProbeClient();
        var session = new DesktopOperationSession();
        session.Attach(transport);
        var result = await ContextualFailureAssistant.CheckLocalAsync(session, CancellationToken.None);
        CollectionAssert.AreEqual(new[] { "check_certificates", "clock_diagnostics" }, transport.Actions.ToArray());
        Assert.IsNotNull(result.Certificates);
        Assert.IsNotNull(result.Clock);
    }

    [TestMethod]
    public async Task FailedCheckDoesNotPreventIndependentClockReading()
    {
        var transport = new LocalProbeClient { FailCertificates = true };
        var session = new DesktopOperationSession();
        session.Attach(transport);
        var result = await ContextualFailureAssistant.CheckLocalAsync(session, CancellationToken.None);
        Assert.IsNull(result.Certificates);
        Assert.IsNotNull(result.Clock);
    }

    [TestMethod]
    public async Task DisconnectedSessionDoesNotInventEvidence()
    {
        var result = await ContextualFailureAssistant.CheckLocalAsync(new(), CancellationToken.None);
        Assert.IsNull(result.Certificates);
        Assert.IsNull(result.Clock);
    }

    [TestMethod]
    public async Task CancellationStopsBeforeAnyRequest()
    {
        var transport = new LocalProbeClient();
        var session = new DesktopOperationSession();
        session.Attach(transport);
        using var cancellation = new CancellationTokenSource();
        cancellation.Cancel();
        await Assert.ThrowsExactlyAsync<OperationCanceledException>(() =>
            ContextualFailureAssistant.CheckLocalAsync(session, cancellation.Token));
        Assert.AreEqual(0, transport.Actions.Count);
    }

    private static ClockDiagnosticResult Clock() => new()
    {
        ThresholdSeconds = 5,
        Steps = [new() { Code = "local_clock", Status = DiagnosticStepStatus.Success },
            new() { Code = "remote_clock", Status = DiagnosticStepStatus.Unknown },
            new() { Code = "government_afirma", Status = DiagnosticStepStatus.Unknown }],
    };

    private sealed class LocalProbeClient : IIpcClient
    {
        public List<string> Actions { get; } = [];
        public bool FailCertificates { get; init; }
        public IpcHello? ServerHello { get; } = new()
        {
            Actions = ["check_certificates", "clock_diagnostics", "sign", "tls_diagnostics"],
        };
        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(string action,
            TParameters parameters, CancellationToken cancellationToken = default)
        {
            cancellationToken.ThrowIfCancellationRequested();
            Actions.Add(action);
            Assert.IsTrue(parameters is CheckCertificatesParameters or ClockDiagnosticsParameters);
            if (action == "check_certificates" && FailCertificates)
                throw new IOException();
            object data = action == "check_certificates"
                ? new CheckCertificatesSummaryResult { OkCount = 2, FailCount = 1 } : Clock();
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name, RequestId = "request-27", TraceId = "trace-27",
                Action = action, IsSuccess = true, Outcome = "success", Data = (TData)data,
            });
        }
        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }
}
