// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Ipc;

public sealed class IpcClientException : Exception
{
    private IpcClientException(
        string code,
        string userMessageKey,
        string phase,
        string likelyOwner,
        bool retryable = false,
        string? reason = null)
        : base(CatalogLocalizer.Shared.Text(userMessageKey))
    {
        Code = code;
        Reason = reason;
        UserMessage = Message;
        Phase = phase;
        LikelyOwner = likelyOwner;
        Retryable = retryable;
    }

    public string Code { get; }
    public string UserMessage { get; }
    public string Phase { get; }
    public string LikelyOwner { get; }
    public bool Retryable { get; }

    /// <summary>
    /// Código técnico de lo que no cuadró en la respuesta (solo ASCII, sin
    /// datos de la operación). Llega al diagnóstico como código del fallo para
    /// que soporte sepa qué forma recibió la interfaz.
    /// </summary>
    public string? Reason { get; }

    internal static IpcClientException Configuration() =>
        new(
            "invalid_configuration",
            "winui.ipc.configuracion_no_valida",
            "admission",
            "app_local");

    internal static IpcClientException Connection(bool retryable = true) =>
        new(
            "connection_failed",
            "winui.ipc.conexion_fallida",
            "admission",
            "unknown",
            retryable);

    internal static IpcClientException ServerIdentity() =>
        new(
            "server_identity_mismatch",
            "winui.ipc.identidad_no_coincide",
            "admission",
            "environment");

    internal static IpcClientException Protocol() =>
        new(
            "protocol_violation",
            "winui.ipc.respuesta_no_valida",
            "protocol",
            "app_local");

    // La respuesta llega bien formada y correlacionada, pero sus campos se
    // contradicen (p. ej. ok:true con errorCode). Suele ser un motor de otra
    // versión: se dice así y se conserva el motivo técnico.
    internal static IpcClientException InconsistentResponse(string reason) =>
        new(
            "protocol_violation",
            "winui.ipc.versiones_incompatibles",
            "protocol",
            "app_local",
            reason: reason);

    internal static IpcClientException ProtocolVersion() =>
        new(
            "unsupported_protocol",
            "winui.ipc.versiones_incompatibles",
            "protocol",
            "app_local");

    internal static IpcClientException FrameTooLarge() =>
        new(
            "message_too_large",
            "winui.ipc.mensaje_demasiado_grande",
            "protocol",
            "app_local");

    internal static IpcClientException Closed(bool retryable = true) =>
        new(
            "connection_closed",
            "winui.ipc.conexion_cerrada",
            "admission",
            "unknown",
            retryable);

    internal static IpcClientException ClosedAfterRequest() =>
        new(
            "connection_closed",
            "winui.ipc.cerrada_tras_peticion",
            "protocol",
            "unknown",
            true);
}
