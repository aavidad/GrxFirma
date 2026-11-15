// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Operations;

public sealed record UnsafeExceptionState
{
    [JsonPropertyName("id")] public string Id { get; init; } = string.Empty;
    [JsonPropertyName("enabled")] public bool Enabled { get; init; }
    [JsonPropertyName("fixed")] public bool Fixed { get; init; }
    [JsonPropertyName("policy_unreadable")] public bool PolicyUnreadable { get; init; }
    [JsonPropertyName("activable")] public bool Activable { get; init; }
    [JsonPropertyName("acceptance_version")] public string AcceptanceVersion { get; init; } = string.Empty;
    [JsonPropertyName("last_acceptance")] public UnsafeAcceptance? LastAcceptance { get; init; }
    // Claves del catálogo que envía el motor (excepciones.Exception en Go).
    // La interfaz no compone claves: solo presenta las que el motor nombra.
    [JsonPropertyName("setting_key")] public string SettingKey { get; init; } = string.Empty;
    [JsonPropertyName("risk_key")] public string RiskKey { get; init; } = string.Empty;
    [JsonPropertyName("acceptance_key")] public string AcceptanceKey { get; init; } = string.Empty;
    [JsonPropertyName("help_key")] public string HelpKey { get; init; } = string.Empty;
    [JsonPropertyName("organization_key")] public string OrganizationKey { get; init; } = string.Empty;
    [JsonIgnore] public bool OperationPending { get; init; }
    public string StatusKey => PolicyUnreadable ? "excepciones.pantalla.directiva_ilegible" : Fixed
        ? Enabled ? "excepciones.pantalla.fijada_activa" : "excepciones.pantalla.fijada_inactiva"
        : !Enabled ? "excepciones.pantalla.desactivada"
        : LastAcceptance is null ? "excepciones.pantalla.activada" : "excepciones.pantalla.activada_fecha";
}

public sealed record UnsafeAcceptance
{
    [JsonPropertyName("accepted_at")] public DateTimeOffset AcceptedAt { get; init; }
    [JsonPropertyName("text_version")] public string TextVersion { get; init; } = string.Empty;
}

// The same acceptance gate drives preferences and blocked operations. Acceptance
// is reset for each presentation; the server remains authoritative for policy.
public sealed class UnsafeExceptionConsentViewModel(UnsafeExceptionState item)
{
    public UnsafeExceptionState Item { get; } = item;
    public bool Accepted { get; set; }
    public bool Busy { get; set; }
    public bool CanActivate => Item.Activable && !Item.Fixed &&
        !string.IsNullOrWhiteSpace(Item.AcceptanceVersion) && Accepted && !Busy;

    // Solo se habla de bloqueo y de continuar cuando el diálogo nace de una
    // operación detenida. Desde Configuración no hay nada que continuar: el
    // título invita a activar y el botón dice «Activar».
    public string TitleKey(bool helpOnly) => helpOnly ? "excepciones.mas_informacion"
        : Item.OperationPending ? "excepciones.pantalla.titulo" : "excepciones.pantalla.aceptar_titulo";

    public string PrimaryButtonKey => Item.OperationPending
        ? "excepciones.activar_continuar" : "excepciones.pantalla.activar";
}

public sealed record UnsafeActivationParameters(
    [property: JsonPropertyName("id")] string Id,
    [property: JsonPropertyName("accepted")] bool Accepted,
    [property: JsonPropertyName("acceptanceVersion")] string AcceptanceVersion,
    [property: JsonPropertyName("retryId"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] string? RetryId);

/// <summary>One IPC boundary for all pages; only a server-issued retry token can
/// replay an operation. No password, PIN or operation parameters are cached.</summary>
public sealed class UnsafeExceptionsClient(IIpcClient inner) : IIpcClient
{
    public Func<UnsafeExceptionState, CancellationToken, Task<bool>>? RequestAcceptance { get; set; }
    public Action<string>? Notice { get; set; }
    public Action<IReadOnlyList<string>>? Warnings { get; set; }
    /// <summary>Retira el aviso común de la operación anterior.</summary>
    public Action? ClearNotice { get; set; }
    public IpcHello? ServerHello => inner.ServerHello;

    // Operaciones con documento cuyo resultado puede llevar el aviso común
    // («…porque usted lo permitió»). Al empezar una, el aviso de la anterior
    // ya no describe lo que hay en pantalla. Las consultas de fondo (ajustes,
    // certificados, estado) no lo retiran.
    public static IReadOnlySet<string> NoticeOperations { get; } = new HashSet<string>(StringComparer.Ordinal)
    {
        DesktopOperationActions.Sign, DesktopOperationActions.CosignOriginalCheck,
        DesktopOperationActions.SignMultiCosign, DesktopOperationActions.SignBatch,
        DesktopOperationActions.Verify, DesktopOperationActions.Protect,
        DesktopOperationActions.ProtectAndSign, DesktopOperationActions.Unprotect,
        DesktopOperationActions.HashCreate, DesktopOperationActions.HashCheck,
        DesktopOperationActions.ValidateInvoice, DesktopOperationActions.ValidateVeriFactu,
        DesktopOperationActions.FacturaeCreate, DesktopOperationActions.GenerateEniDocument,
        DesktopOperationActions.GenerateEniFile,
    };

    public Task<IpcCallResult<IReadOnlyList<UnsafeExceptionState>>> ListAsync(CancellationToken ct = default) =>
        inner.SendAsync<object, IReadOnlyList<UnsafeExceptionState>>("unsafe_exceptions_list", new { }, ct);

    public Task<IpcCallResult<IReadOnlyList<UnsafeExceptionState>>> DisableAsync(UnsafeExceptionState item, CancellationToken ct = default)
    {
        if (item.Fixed) throw new InvalidOperationException();
        return DisableCoreAsync(item, ct);
    }

    private async Task<IpcCallResult<IReadOnlyList<UnsafeExceptionState>>> DisableCoreAsync(
        UnsafeExceptionState item, CancellationToken ct)
    {
        var result = await inner.SendAsync<object, IReadOnlyList<UnsafeExceptionState>>(
            "unsafe_exceptions_set", new { id = item.Id, enabled = false }, ct).ConfigureAwait(false);
        // Tras desactivar, el aviso de que se usó lo inseguro ya no es cierto.
        if (result.IsSuccess) ClearNotice?.Invoke();
        return result;
    }

    public async Task<bool> ActivateAsync(UnsafeExceptionState item, string? retryId = null, CancellationToken ct = default)
    {
        if (!item.Activable || item.Fixed || string.IsNullOrWhiteSpace(item.AcceptanceVersion) || RequestAcceptance is null)
            return false;
        if (!await RequestAcceptance(item, ct).ConfigureAwait(false)) return false;
        ct.ThrowIfCancellationRequested();
        var result = await inner.SendAsync<UnsafeActivationParameters, JsonElement>("unsafe_exceptions_activate",
            new(item.Id, true, item.AcceptanceVersion, retryId), ct).ConfigureAwait(false);
        if (result.IsSuccess) ClearNotice?.Invoke();
        else Notice?.Invoke("excepciones.pantalla.error");
        return result.IsSuccess;
    }

    public async Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(string action,
        TParameters parameters, CancellationToken cancellationToken = default)
    {
        if (NoticeOperations.Contains(action)) ClearNotice?.Invoke();
        var result = await inner.SendAsync<TParameters, TData>(action, parameters, cancellationToken).ConfigureAwait(false);
        // Multiple independent gates (e.g. old encryption and old key transport)
        // each need their own explicit acceptance. Never automatically accept.
        for (var count = 0; count < 26 && !result.IsSuccess && result.UnsafeException is { } block; count++)
        {
            if (!block.Activable)
            {
                if (RequestAcceptance is not null) await RequestAcceptance(block, cancellationToken).ConfigureAwait(false);
                break;
            }
            if (!await ActivateAsync(block, result.RetryId, cancellationToken).ConfigureAwait(false)) break;
            if (string.IsNullOrWhiteSpace(result.RetryId))
            {
                Notice?.Invoke("excepciones.pantalla.reenviar");
                break;
            }
            var retried = await inner.SendAsync<object, TData>("unsafe_operation_retry",
                new { retryId = result.RetryId }, cancellationToken).ConfigureAwait(false);
            result = retried with { Action = action };
        }
        if (result.IsSuccess)
        {
            var keys = result.WarningKeys.ToList();
            keys.AddRange(BodyWarnings(result.Data));
            if (keys.Count > 0) Warnings?.Invoke(keys.Distinct(StringComparer.Ordinal).Take(32).ToArray());
        }
        return result;
    }
    private static IEnumerable<string> BodyWarnings(object? data) => data switch
    {
        SignResult sign => sign.VisibleWarnings,
        BatchSignResult batch => batch.VisibleWarnings.Concat(batch.Results.SelectMany(item => item.VisibleWarnings)),
        VerifyResult verify => verify.VisibleWarnings,
        ProtectionResult protect => protect.VisibleWarnings,
        UnprotectResult unprotect => unprotect.VisibleWarnings,
        JsonElement json when json.ValueKind == JsonValueKind.Object &&
            json.TryGetProperty("warnings", out var warnings) && warnings.ValueKind == JsonValueKind.Array =>
            warnings.EnumerateArray().Where(x => x.ValueKind == JsonValueKind.String).Select(x => x.GetString()!),
        _ => [],
    };

    // Ownership of the transport stays with App, which also watches reconnects.
    public ValueTask DisposeAsync() => ValueTask.CompletedTask;
}
