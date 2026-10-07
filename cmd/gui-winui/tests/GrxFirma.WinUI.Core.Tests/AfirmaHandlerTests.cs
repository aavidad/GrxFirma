// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class AfirmaHandlerTests
{
    [TestMethod]
    public void Status_DeserializesEngineContract()
    {
        var status = JsonSerializer.Deserialize<AfirmaHandlerStatus>("""
            {"supported":true,"current":"autofirma","currentPath":"C:\\Program Files\\Autofirma\\Autofirma\\Autofirma.exe",
             "grxfirmaInstalled":true,"autofirmaInstalled":true,"autofirmaPath":"C:\\Program Files\\Autofirma\\Autofirma\\Autofirma.exe",
             "preference":"autofirma"}
            """)!;
        Assert.IsTrue(status.Supported);
        Assert.AreEqual(AfirmaHandlerChoices.AutoFirma, status.Current);
        Assert.IsTrue(status.AutoFirmaInstalled);
        Assert.AreEqual("autofirma", status.Preference);
    }

    [TestMethod]
    public void View_ShowsRealStateAndDisablesMissingAutoFirma()
    {
        var grx = AfirmaHandlerPresentation.From(new AfirmaHandlerStatus
        {
            Supported = true,
            Current = AfirmaHandlerChoices.GrxFirma,
            GrxFirmaInstalled = true,
        });
        Assert.IsTrue(grx.Visible);
        Assert.IsTrue(grx.GrxFirmaSelected);
        Assert.IsFalse(grx.AutoFirmaSelected);
        Assert.IsFalse(grx.CanChooseAutoFirma);
        Assert.IsTrue(grx.ShowAutoFirmaMissing);
        Assert.AreEqual("protocolo.estado.grxfirma", grx.StatusKey);

        var auto = AfirmaHandlerPresentation.From(new AfirmaHandlerStatus
        {
            Supported = true,
            Current = AfirmaHandlerChoices.AutoFirma,
            GrxFirmaInstalled = true,
            AutoFirmaInstalled = true,
        });
        Assert.IsTrue(auto.AutoFirmaSelected);
        Assert.IsTrue(auto.CanChooseAutoFirma);
        Assert.IsFalse(auto.ShowAutoFirmaMissing);
        Assert.AreEqual("protocolo.estado.autofirma", auto.StatusKey);

        var other = AfirmaHandlerPresentation.From(new AfirmaHandlerStatus
        {
            Supported = true,
            Current = AfirmaHandlerChoices.Other,
            CurrentPath = " C:\\Otro\\firmador.exe\u202e ",
        });
        Assert.IsFalse(other.GrxFirmaSelected || other.AutoFirmaSelected);
        Assert.AreEqual("protocolo.estado.otro", other.StatusKey);
        Assert.AreEqual("C:\\Otro\\firmador.exe", other.StatusArgument);

        Assert.AreEqual("protocolo.estado.ninguno",
            AfirmaHandlerPresentation.From(new AfirmaHandlerStatus { Supported = true }).StatusKey);
        Assert.IsFalse(AfirmaHandlerPresentation.From(null).Visible);
        Assert.IsFalse(AfirmaHandlerPresentation.From(new AfirmaHandlerStatus()).Visible);
    }

    [TestMethod]
    public void ErrorKey_MapsStableCodesToPlainMessages()
    {
        Assert.AreEqual("protocolo.error.ajeno", AfirmaHandlerPresentation.ErrorKey("afirma_handler_foreign"));
        Assert.AreEqual("protocolo.autofirma_no_instalada", AfirmaHandlerPresentation.ErrorKey("autofirma_not_installed"));
        Assert.AreEqual("protocolo.error.grxfirma_no_instalada", AfirmaHandlerPresentation.ErrorKey("grxfirma_afirma_missing"));
        Assert.AreEqual("protocolo.error.generico", AfirmaHandlerPresentation.ErrorKey("operation_failed"));
        Assert.AreEqual("protocolo.error.generico", AfirmaHandlerPresentation.ErrorKey(null));
    }

    [TestMethod]
    public async Task Client_SendsOnlySelectableHandlers()
    {
        var ipc = new RecordingIpcClient();
        var client = new DesktopOperationsClient(ipc);
        await client.GetAfirmaHandlerStatusAsync();
        await client.SelectAfirmaHandlerAsync(AfirmaHandlerChoices.AutoFirma);
        Assert.AreEqual("afirma_handler_status", ipc.Actions[0]);
        Assert.AreEqual("afirma_handler_select", ipc.Actions[1]);
        Assert.AreEqual("""{"handler":"autofirma"}""", ipc.Parameters[1]);
        await Assert.ThrowsExactlyAsync<ArgumentOutOfRangeException>(
            () => client.SelectAfirmaHandlerAsync("other"));
        await Assert.ThrowsExactlyAsync<ArgumentOutOfRangeException>(
            () => client.SelectAfirmaHandlerAsync("C:\\malware.exe"));
        Assert.AreEqual(2, ipc.Actions.Count);
    }

    private sealed class RecordingIpcClient : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public List<string> Actions { get; } = [];
        public List<string> Parameters { get; } = [];

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default)
        {
            Actions.Add(action);
            Parameters.Add(JsonSerializer.Serialize(parameters));
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "request-afirma-1",
                TraceId = "trace-afirma-1",
                Action = action,
                IsSuccess = true,
                Outcome = "success",
                Phase = "operation",
            });
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }
}
