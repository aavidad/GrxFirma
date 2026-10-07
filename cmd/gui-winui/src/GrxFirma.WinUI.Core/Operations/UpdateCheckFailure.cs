// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Net;
using System.Net.Sockets;
using System.Security.Authentication;
using System.Text.Json;
using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Operations;

public enum UpdateCheckFailureKind
{
    ProxyAuthenticationRequired,
    Proxy,
    HttpStatus,
    Timeout,
    NameResolution,
    Network,
    Tls,
    InvalidResponse,
    Unknown,
}

/// <summary>
/// Motivo de un fallo al consultar la versión publicada. Solo guarda el tipo y
/// el código HTTP: nunca la URL del proxy, credenciales ni texto remoto, de
/// modo que <see cref="LogCode"/> puede escribirse tal cual en el registro.
/// </summary>
public sealed record UpdateCheckFailure(UpdateCheckFailureKind Kind, int? StatusCode = null)
{
    // El mensaje del túnel CONNECT termina en «... status code '407'.»; la
    // URL del proxy que también lleva no se lee ni se guarda.
    private static readonly Regex TunnelStatus = new(
        @"(?:'(?<code>[1-5][0-9]{2})')\.?\s*$",
        RegexOptions.CultureInvariant | RegexOptions.NonBacktracking);

    public string LogCode => StatusCode is int code ? $"{KindCode} {code}" : KindCode;

    private string KindCode => Kind switch
    {
        UpdateCheckFailureKind.ProxyAuthenticationRequired => "proxy_auth_required",
        UpdateCheckFailureKind.Proxy => "proxy_error",
        UpdateCheckFailureKind.HttpStatus => "http_status",
        UpdateCheckFailureKind.Timeout => "request_timeout",
        UpdateCheckFailureKind.NameResolution => "dns_error",
        UpdateCheckFailureKind.Network => "network_error",
        UpdateCheckFailureKind.Tls => "tls_error",
        UpdateCheckFailureKind.InvalidResponse => "invalid_response",
        _ => "unknown_error",
    };

    /// <summary>Clave de catálogo del mensaje para el usuario.</summary>
    public string MessageKey => Kind switch
    {
        UpdateCheckFailureKind.ProxyAuthenticationRequired or UpdateCheckFailureKind.Proxy =>
            "winui.actualizaciones.error_proxy",
        UpdateCheckFailureKind.HttpStatus => "winui.actualizaciones.error_servicio",
        UpdateCheckFailureKind.Timeout => "winui.actualizaciones.error_tiempo",
        UpdateCheckFailureKind.NameResolution or UpdateCheckFailureKind.Network =>
            "winui.actualizaciones.error_red",
        UpdateCheckFailureKind.Tls => "winui.actualizaciones.error_tls",
        _ => "winui.actualizaciones.error_otro",
    };

    public static UpdateCheckFailure Classify(Exception error)
    {
        ArgumentNullException.ThrowIfNull(error);
        for (Exception? current = error; current is not null; current = current.InnerException)
        {
            switch (current)
            {
                case OfficialUpdateCheckException official:
                    return official.Failure;
                case HttpRequestException http:
                    var status = http.StatusCode is { } value ? (int)value : (int?)null;
                    if (http.HttpRequestError == HttpRequestError.ProxyTunnelError)
                    {
                        status ??= TunnelStatusCode(http.Message);
                        return status == (int)HttpStatusCode.ProxyAuthenticationRequired
                            ? new(UpdateCheckFailureKind.ProxyAuthenticationRequired, status)
                            : new(UpdateCheckFailureKind.Proxy, status);
                    }
                    if (status == (int)HttpStatusCode.ProxyAuthenticationRequired)
                        return new(UpdateCheckFailureKind.ProxyAuthenticationRequired, status);
                    switch (http.HttpRequestError)
                    {
                        case HttpRequestError.NameResolutionError:
                            return new(UpdateCheckFailureKind.NameResolution);
                        case HttpRequestError.SecureConnectionError:
                            return new(UpdateCheckFailureKind.Tls);
                        case HttpRequestError.ConnectionError when current.InnerException is not
                            (SocketException or IOException or TimeoutException):
                            return new(UpdateCheckFailureKind.Network);
                    }
                    if (status is int code) return new(UpdateCheckFailureKind.HttpStatus, code);
                    break;
                case AuthenticationException:
                    return new(UpdateCheckFailureKind.Tls);
                case TimeoutException or OperationCanceledException:
                    return new(UpdateCheckFailureKind.Timeout);
                case SocketException socket:
                    return socket.SocketErrorCode switch
                    {
                        SocketError.HostNotFound or SocketError.TryAgain or SocketError.NoData =>
                            new(UpdateCheckFailureKind.NameResolution),
                        SocketError.TimedOut => new(UpdateCheckFailureKind.Timeout),
                        _ => new(UpdateCheckFailureKind.Network),
                    };
                case InvalidDataException or JsonException:
                    return new(UpdateCheckFailureKind.InvalidResponse);
            }
        }
        return error is HttpRequestException or IOException
            ? new(UpdateCheckFailureKind.Network)
            : new(UpdateCheckFailureKind.Unknown);
    }

    private static int? TunnelStatusCode(string? message)
    {
        var match = TunnelStatus.Match(message ?? string.Empty);
        return match.Success ? int.Parse(match.Groups["code"].Value, System.Globalization.CultureInfo.InvariantCulture) : null;
    }
}

/// <summary>Fallo ya clasificado de la consulta de versiones.</summary>
public sealed class OfficialUpdateCheckException : Exception
{
    public OfficialUpdateCheckException(UpdateCheckFailure failure, Exception? inner = null)
        : base(failure?.LogCode, inner)
    {
        ArgumentNullException.ThrowIfNull(failure);
        Failure = failure;
    }

    public UpdateCheckFailure Failure { get; }
}
