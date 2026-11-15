// SPDX-License-Identifier: EUPL-1.2
using System.Text.Json;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using Microsoft.VisualStudio.TestTools.UnitTesting;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class UnsafeExceptionsTests
{
    private static UnsafeExceptionState Item(bool activable = true, bool fixedValue = false) => new()
    { Id = "cms_cbc", Activable = activable, Fixed = fixedValue, AcceptanceVersion = "2026-11-15.1" };

    [TestMethod]
    public void Consent_DisabledUntilChecked_AndEveryPresentationStartsUnchecked()
    {
        var model = new UnsafeExceptionConsentViewModel(Item());
        Assert.IsFalse(model.CanActivate);
        model.Accepted = true;
        Assert.IsTrue(model.CanActivate);
        model.Busy = true;
        Assert.IsFalse(model.CanActivate);
        Assert.IsFalse(new UnsafeExceptionConsentViewModel(Item()).CanActivate);
        Assert.IsFalse(new UnsafeExceptionConsentViewModel(Item(false)) { Accepted = true }.CanActivate);
        Assert.IsFalse(new UnsafeExceptionConsentViewModel(Item() with { AcceptanceVersion = "" }) { Accepted = true }.CanActivate);
    }

    [TestMethod]
    public async Task Activation_SendsAcceptedVersion_AndRetryUsesOnlyToken()
    {
        var transport = new Fake();
        var client = new UnsafeExceptionsClient(transport) { RequestAcceptance = (_, _) => Task.FromResult(true) };
        var result = await client.SendAsync<object, JsonElement>("verify", new { inputPath = "fixture.pdf" });
        Assert.IsTrue(result.IsSuccess);
        Assert.AreEqual("verify", result.Action);
        CollectionAssert.AreEqual(new[] { "verify", "unsafe_exceptions_activate", "unsafe_operation_retry" }, transport.Calls.Select(x => x.Action).ToArray());
        var activation = transport.Calls[1].Parameters;
        Assert.IsTrue(activation.GetProperty("accepted").GetBoolean());
        Assert.AreEqual("2026-11-15.1", activation.GetProperty("acceptanceVersion").GetString());
        Assert.AreEqual("cms_cbc", activation.GetProperty("id").GetString());
        Assert.AreEqual("fixture-token", activation.GetProperty("retryId").GetString());
        Assert.AreEqual(1, transport.Calls[2].Parameters.EnumerateObject().Count());
    }

    [TestMethod]
    public async Task PolicyBlock_IsPresentedWithoutActivation_AndPreferencesAreReadOnly()
    {
        var transport = new Fake { Block = Item(false, true) };
        var shown = 0;
        var client = new UnsafeExceptionsClient(transport) { RequestAcceptance = (item, _) => { shown++; Assert.IsFalse(item.Activable); return Task.FromResult(true); } };
        await client.SendAsync<object, JsonElement>("protect", new { });
        Assert.AreEqual(1, shown);
        Assert.AreEqual(1, transport.Calls.Count);
        Assert.IsFalse(await client.ActivateAsync(transport.Block));
        await Assert.ThrowsExactlyAsync<InvalidOperationException>(() => client.DisableAsync(transport.Block));
        Assert.AreEqual("excepciones.pantalla.fijada_inactiva", transport.Block.StatusKey);
        Assert.AreEqual("excepciones.pantalla.directiva_ilegible", (transport.Block with { PolicyUnreadable = true }).StatusKey);
    }

    [TestMethod]
    public async Task Cancel_OrFailedActivation_NeverRetries()
    {
        var transport = new Fake();
        var client = new UnsafeExceptionsClient(transport) { RequestAcceptance = (_, _) => Task.FromResult(false) };
        await client.SendAsync<object, JsonElement>("sign", new { });
        Assert.AreEqual(1, transport.Calls.Count);
        transport.Calls.Clear();
        transport.ActivationSucceeds = false;
        client.RequestAcceptance = (_, _) => Task.FromResult(true);
        string? notice = null;
        client.Notice = value => notice = value;
        await client.SendAsync<object, JsonElement>("sign", new { });
        Assert.AreEqual(2, transport.Calls.Count);
        Assert.AreEqual("excepciones.pantalla.error", notice);
    }

    [TestMethod]
    public async Task NoToken_PreservesFormWithoutReplayingParameters()
    {
        var transport = new Fake { RetryId = null };
        var client = new UnsafeExceptionsClient(transport) { RequestAcceptance = (_, _) => Task.FromResult(true) };
        string? notice = null;
        client.Notice = value => notice = value;
        await client.SendAsync<object, JsonElement>("protect", new { passwordB64 = "dummy" });
        Assert.AreEqual(2, transport.Calls.Count);
        Assert.IsFalse(transport.Calls[1].Parameters.TryGetProperty("retryId", out _));
        Assert.AreEqual("excepciones.pantalla.reenviar", notice);
    }

    [TestMethod]
    public async Task Preferences_ListAndDirectDisable_AndAcceptedDate()
    {
        var transport = new Fake();
        var client = new UnsafeExceptionsClient(transport);
        var result = await client.ListAsync();
        Assert.AreEqual(1, result.Data!.Count);
        await client.DisableAsync(Item() with { Enabled = true });
        Assert.AreEqual("unsafe_exceptions_set", transport.Calls[1].Action);
        Assert.IsFalse(transport.Calls[1].Parameters.GetProperty("enabled").GetBoolean());
        Assert.AreEqual("excepciones.pantalla.activada_fecha", (Item() with { Enabled = true,
            LastAcceptance = new UnsafeAcceptance { AcceptedAt = DateTimeOffset.Parse("2026-11-15T10:00:00Z") } }).StatusKey);
    }

    [TestMethod]
    public async Task RetryWarnings_ReachOperationNotice()
    {
        var transport = new Fake();
        IReadOnlyList<string>? warnings = null;
        var client = new UnsafeExceptionsClient(transport) { RequestAcceptance = (_, _) => Task.FromResult(true), Warnings = value => warnings = value };
        await client.SendAsync<object, JsonElement>("verify", new { });
        Assert.IsNotNull(warnings);
        CollectionAssert.Contains(warnings.ToArray(), "excepciones.cms_cbc.aviso");
    }

    [TestMethod]
    public void Wire_DeserializesBlockedMetadataAndWarnings()
    {
        var wire = JsonSerializer.Deserialize<IpcWireResponse<JsonElement>>("""
            {"ok":false,"exceptionId":"cms_cbc","activable":false,"retryId":"token",
             "acceptanceVersion":"2026-11-15.1","warnings":["excepciones.cms_cbc.aviso"],
             "exception":{"id":"cms_cbc","activable":false,"acceptance_version":"2026-11-15.1"}}
            """);
        Assert.IsNotNull(wire);
        Assert.IsNotNull(wire.UnsafeException);
        Assert.IsFalse(wire.UnsafeException.Activable);
        Assert.AreEqual("2026-11-15.1", wire.UnsafeException.AcceptanceVersion);
        Assert.AreEqual("token", wire.RetryId);
        Assert.AreEqual("excepciones.cms_cbc.aviso", wire.WarningKeys[0]);
    }

    private sealed class Fake : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public UnsafeExceptionState Block { get; init; } = Item();
        public bool ActivationSucceeds { get; set; } = true;
        public string? RetryId { get; init; } = "fixture-token";
        public List<(string Action, JsonElement Parameters)> Calls { get; } = [];
        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(string action, TParameters parameters, CancellationToken cancellationToken = default)
        {
            Calls.Add((action, JsonSerializer.SerializeToElement(parameters)));
            var control = action.StartsWith("unsafe_", StringComparison.Ordinal);
            var ok = control && (action != "unsafe_exceptions_activate" || ActivationSucceeds);
            object data = action is "unsafe_exceptions_list" or "unsafe_exceptions_set" ? new[] { Block }
                : action == "unsafe_operation_retry" ? new { warnings = new[] { "excepciones.cms_cbc.aviso" } } : new { };
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name, RequestId = "request", TraceId = "trace", Action = action,
                IsSuccess = ok, Outcome = ok ? "success" : "failure", ErrorCode = ok ? null : "unsafe_exception_blocked",
                UnsafeException = control ? null : Block, RetryId = control ? null : RetryId,
                Data = ok ? JsonSerializer.Deserialize<TData>(JsonSerializer.Serialize(data)) : default,
            });
        }
        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }
}
