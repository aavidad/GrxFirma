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
        string userMessage,
        string phase,
        string likelyOwner,
        bool retryable = false)
        : base(CatalogLocalizer.Shared.TranslateVisibleText(userMessage))
    {
        Code = code;
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

    internal static IpcClientException Configuration() =>
        new(
            "invalid_configuration",
            "La configuración de conexión local no es válida.",
            "admission",
            "app_local");

    internal static IpcClientException Connection(bool retryable = true) =>
        new(
            "connection_failed",
            "No se pudo conectar de forma segura con el motor local.",
            "admission",
            "unknown",
            retryable);

    internal static IpcClientException ServerIdentity() =>
        new(
            "server_identity_mismatch",
            "Se rechazó la conexión porque el motor local no coincide con el proceso esperado.",
            "admission",
            "environment");

    internal static IpcClientException Protocol() =>
        new(
            "protocol_violation",
            "El motor local devolvió una respuesta no válida.",
            "protocol",
            "app_local");

    internal static IpcClientException ProtocolVersion() =>
        new(
            "unsupported_protocol",
            "La interfaz y el motor local usan versiones incompatibles.",
            "protocol",
            "app_local");

    internal static IpcClientException FrameTooLarge() =>
        new(
            "message_too_large",
            "Un mensaje del canal local supera el límite permitido.",
            "protocol",
            "app_local");

    internal static IpcClientException Closed(bool retryable = true) =>
        new(
            "connection_closed",
            "La conexión segura con el motor local se cerró antes de completar la operación.",
            "admission",
            "unknown",
            retryable);

    internal static IpcClientException ClosedAfterRequest() =>
        new(
            "connection_closed",
            "El canal local se cerró después de enviar la petición y antes de recibir una respuesta.",
            "protocol",
            "unknown",
            true);
}
