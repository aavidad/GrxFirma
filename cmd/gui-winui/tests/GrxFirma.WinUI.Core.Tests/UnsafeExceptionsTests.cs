// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
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
        Assert.AreEqual("aviso", activation.GetProperty("origin").GetString());
        Assert.AreEqual(1, transport.Calls[2].Parameters.EnumerateObject().Count());
    }

    // Lote 39f: activar una excepción no confirma una firma. El motor puede
    // ofrecer un reintento, pero una firma no se repite sin que la persona la
    // vuelva a lanzar y pase otra vez por la confirmación de firma.
    [TestMethod]
    public async Task SignatureBlock_ActivatesWithoutReplayingTheSignature()
    {
        foreach (var action in new[] { "sign", "sign_batch", "sign_multicosign", "protect_sign" })
        {
            var transport = new Fake();
            string? notice = null;
            var client = new UnsafeExceptionsClient(transport)
            { RequestAcceptance = (_, _) => Task.FromResult(true), Notice = value => notice = value };
            var result = await client.SendAsync<object, JsonElement>(action, new { inputPath = "fixture.pdf" });
            Assert.IsFalse(result.IsSuccess, action);
            CollectionAssert.AreEqual(new[] { action, "unsafe_exceptions_activate" },
                transport.Calls.Select(x => x.Action).ToArray(), action);
            var activation = transport.Calls[1].Parameters;
            Assert.IsFalse(activation.TryGetProperty("retryId", out _), action);
            Assert.AreEqual("aviso", activation.GetProperty("origin").GetString(), action);
            Assert.AreEqual("excepciones.pantalla.reenviar", notice, action);
        }
    }

    // Lote 39f: el recibo distingue Configuración del aviso de un bloqueo.
    [TestMethod]
    public async Task Activation_FromSettingsSaysAjustes_FromABlockSaysAviso()
    {
        var transport = new Fake();
        var client = new UnsafeExceptionsClient(transport) { RequestAcceptance = (_, _) => Task.FromResult(true) };
        Assert.IsTrue(await client.ActivateAsync(Item()));
        Assert.AreEqual("ajustes", transport.Calls[0].Parameters.GetProperty("origin").GetString());
        Assert.IsFalse(transport.Calls[0].Parameters.TryGetProperty("retryId", out _));
        Assert.IsTrue(await client.ActivateAsync(Item() with { OperationPending = true }));
        Assert.AreEqual("aviso", transport.Calls[1].Parameters.GetProperty("origin").GetString());
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
        Assert.AreEqual("aviso", transport.Calls[1].Parameters.GetProperty("origin").GetString());
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
    public async Task NewOperationOrSettingChange_ClearsPreviousNotice_BackgroundQueriesDoNot()
    {
        var transport = new Fake();
        var cleared = 0;
        var client = new UnsafeExceptionsClient(transport)
        { RequestAcceptance = (_, _) => Task.FromResult(false), ClearNotice = () => cleared++ };
        await client.SendAsync<object, JsonElement>("get_settings", new { });
        await client.ListAsync();
        Assert.AreEqual(0, cleared, "Una consulta de fondo no retira el aviso del resultado.");
        await client.SendAsync<object, JsonElement>("verify", new { });
        Assert.AreEqual(1, cleared, "Una verificación nueva retira el aviso de la anterior.");
        await client.DisableAsync(Item() with { Enabled = true });
        Assert.AreEqual(2, cleared, "Desactivar retira «…porque usted lo permitió».");
        client.RequestAcceptance = (_, _) => Task.FromResult(true);
        Assert.IsTrue(await client.ActivateAsync(Item()));
        Assert.AreEqual(3, cleared);
        transport.ActivationSucceeds = false;
        Assert.IsFalse(await client.ActivateAsync(Item()));
        Assert.AreEqual(3, cleared, "Si no se activó, el ajuste no cambió.");
        foreach (var action in new[] { "sign", "sign_batch", "sign_multicosign", "protect", "protect_sign", "unprotect" })
            Assert.IsTrue(UnsafeExceptionsClient.NoticeOperations.Contains(action), action);
    }

    [TestMethod]
    public void Dialog_FromSettingsDoesNotTalkAboutABlock_AndSaysActivar()
    {
        var fromSettings = new UnsafeExceptionConsentViewModel(Item());
        Assert.AreEqual("excepciones.pantalla.aceptar_titulo", fromSettings.TitleKey(helpOnly: false));
        Assert.AreEqual("excepciones.pantalla.activar", fromSettings.PrimaryButtonKey);
        Assert.AreEqual("excepciones.mas_informacion", fromSettings.TitleKey(helpOnly: true));
        var blocked = new UnsafeExceptionConsentViewModel(Item() with { OperationPending = true });
        Assert.AreEqual("excepciones.pantalla.titulo", blocked.TitleKey(helpOnly: false));
        Assert.AreEqual("excepciones.activar_continuar", blocked.PrimaryButtonKey);
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

    [TestMethod]
    public void Wire_TakesCatalogKeysFromTheEngine()
    {
        var listed = JsonSerializer.Deserialize<UnsafeExceptionState>("""
            {"id":"cms_cbc","setting_key":"excepciones.cms_cbc.ajuste","risk_key":"excepciones.cms_cbc.riesgo",
             "acceptance_key":"excepciones.cms_cbc.aceptacion","help_key":"excepciones.cms_cbc.ayuda",
             "organization_key":"excepciones.cms_cbc.organizacion"}
            """);
        Assert.IsNotNull(listed);
        Assert.AreEqual("excepciones.cms_cbc.ajuste", listed.SettingKey);
        Assert.AreEqual("excepciones.cms_cbc.riesgo", listed.RiskKey);
        Assert.AreEqual("excepciones.cms_cbc.aceptacion", listed.AcceptanceKey);
        Assert.AreEqual("excepciones.cms_cbc.ayuda", listed.HelpKey);
        Assert.AreEqual("excepciones.cms_cbc.organizacion", listed.OrganizationKey);

        var wire = JsonSerializer.Deserialize<IpcWireResponse<JsonElement>>("""
            {"ok":false,"exceptionId":"cms_cbc","activable":true,"settingKey":"excepciones.cms_cbc.ajuste",
             "riskKey":"excepciones.cms_cbc.riesgo","acceptanceKey":"excepciones.cms_cbc.aceptacion",
             "helpKey":"excepciones.cms_cbc.ayuda"}
            """);
        Assert.IsNotNull(wire);
        Assert.AreEqual("excepciones.cms_cbc.aceptacion", wire.AcceptanceKey);
        Assert.AreEqual("excepciones.cms_cbc.ajuste", wire.SettingKey);
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
