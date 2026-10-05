// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Buffers;
using System.Globalization;
using System.Text;
using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Operations;

// Firma remota CSC. El motor guarda la sesión con el prestador: la interfaz
// solo recibe el estado, los hosts que debe mostrar antes de abrir el
// navegador y la descripción de los certificados remotos. Nunca tokens.

public sealed record RemoteSigningStatusParameters;

public sealed record RemoteSigningConnectParameters;

public sealed record RemoteSigningDisconnectParameters;

public sealed record RemoteSigningConfigureParameters
{
    [JsonPropertyName("serviceUrl")]
    public required string ServiceUrl { get; init; }

    [JsonPropertyName("clientId")]
    public required string ClientId { get; init; }
}

public sealed record RemoteSigningSendOtpParameters
{
    [JsonPropertyName("certificateId")]
    public required string CertificateId { get; init; }
}

public sealed record RemoteSigningAck;

public sealed record RemoteSigningStatus
{
    private string _serviceUrl = string.Empty;
    private string _clientId = string.Empty;
    private string _serviceHost = string.Empty;
    private string _oauthHost = string.Empty;
    private string _serviceName = string.Empty;

    [JsonPropertyName("allowed")]
    public bool Allowed { get; init; }

    /// <summary>
    /// La política de la organización prohíbe la firma remota: se explica a
    /// la persona y no se le sugiere config.json.
    /// </summary>
    [JsonPropertyName("prohibitedByPolicy")]
    public bool ProhibitedByPolicy { get; init; }

    /// <summary>
    /// Con la firma remota prohibida, la persona la tenía configurada: solo
    /// entonces se muestra el botón, para explicarle la prohibición.
    /// </summary>
    [JsonPropertyName("userConfigured")]
    public bool UserConfigured { get; init; }

    [JsonPropertyName("serviceUrl")]
    public string ServiceUrl
    {
        get => _serviceUrl;
        init => _serviceUrl = OperationResultText.Clean(value, RemoteSigningInput.MaximumServiceUrlLength);
    }

    [JsonPropertyName("clientId")]
    public string ClientId
    {
        get => _clientId;
        init => _clientId = OperationResultText.Clean(value, RemoteSigningInput.MaximumClientIdLength);
    }

    [JsonPropertyName("discovered")]
    public bool Discovered { get; init; }

    [JsonPropertyName("connected")]
    public bool Connected { get; init; }

    [JsonPropertyName("serviceHost")]
    public string ServiceHost
    {
        get => _serviceHost;
        init => _serviceHost = OperationResultText.Clean(value, 300);
    }

    [JsonPropertyName("oauthHost")]
    public string OAuthHost
    {
        get => _oauthHost;
        init => _oauthHost = OperationResultText.Clean(value, 300);
    }

    [JsonPropertyName("serviceName")]
    public string ServiceName
    {
        get => _serviceName;
        init => _serviceName = OperationResultText.Clean(value, 120);
    }
}

public sealed record RemoteSigningDiscovery
{
    private string _serviceHost = string.Empty;
    private string _oauthHost = string.Empty;
    private string _serviceName = string.Empty;

    [JsonPropertyName("serviceHost")]
    public string ServiceHost
    {
        get => _serviceHost;
        init => _serviceHost = OperationResultText.Clean(value, 300);
    }

    [JsonPropertyName("oauthHost")]
    public string OAuthHost
    {
        get => _oauthHost;
        init => _oauthHost = OperationResultText.Clean(value, 300);
    }

    [JsonPropertyName("serviceName")]
    public string ServiceName
    {
        get => _serviceName;
        init => _serviceName = OperationResultText.Clean(value, 120);
    }
}

public sealed record RemoteCredentialInfo
{
    private string _certificateId = string.Empty;
    private string _subject = string.Empty;
    private string _issuer = string.Empty;

    [JsonPropertyName("certificateId")]
    public string CertificateId
    {
        get => _certificateId;
        init => _certificateId = OperationResultText.Clean(value, 128);
    }

    [JsonPropertyName("subject")]
    public string Subject
    {
        get => _subject;
        init => _subject = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("issuer")]
    public string Issuer
    {
        get => _issuer;
        init => _issuer = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("pin")]
    public bool Pin { get; init; }

    [JsonPropertyName("otp")]
    public bool Otp { get; init; }

    [JsonPropertyName("otpOnline")]
    public bool OtpOnline { get; init; }
}

public sealed record RemoteSigningConnection
{
    [JsonPropertyName("credentials")]
    public IReadOnlyList<RemoteCredentialInfo> Credentials { get; init; } = [];

    [JsonPropertyName("omitted")]
    public int Omitted { get; init; }
}

/// <summary>
/// Validación local de lo que la persona escribe. El motor vuelve a validar
/// con las mismas reglas que la CLI; aquí solo se evita enviar algo que no
/// puede ser válido.
/// </summary>
public static class RemoteSigningInput
{
    public const int MaximumServiceUrlLength = 2048;
    public const int MaximumClientIdLength = 256;
    public const int MaximumSecretBytes = 256;

    public static RemoteSigningConfigureParameters Normalize(string? serviceUrl, string? clientId)
    {
        // Los caracteres de control se buscan antes de recortar: Trim() quitaría
        // un salto de línea final que el motor rechaza.
        var rawUrl = serviceUrl ?? string.Empty;
        var url = rawUrl.Trim();
        var id = (clientId ?? string.Empty).Trim();
        if (url.Length is 0 or > MaximumServiceUrlLength ||
            !url.StartsWith("https://", StringComparison.OrdinalIgnoreCase) ||
            rawUrl.Any(char.IsControl) ||
            !Uri.TryCreate(url, UriKind.Absolute, out var uri) ||
            !string.Equals(uri.Scheme, Uri.UriSchemeHttps, StringComparison.OrdinalIgnoreCase) ||
            uri.UserInfo.Length > 0 || uri.Query.Length > 0 || uri.Fragment.Length > 0)
        {
            throw new ArgumentException("csc.error.url_invalida", nameof(serviceUrl));
        }
        if (id.Length is 0 or > MaximumClientIdLength || id.Any(c => c <= ' ' || c > '~'))
        {
            throw new ArgumentException("csc.error.parametro_invalido", nameof(clientId));
        }
        return new RemoteSigningConfigureParameters { ServiceUrl = url, ClientId = id };
    }

    public static bool IsValidCertificateId(string? id) =>
        !string.IsNullOrEmpty(id) && id.Length <= 128 &&
        id.All(c => c is >= '0' and <= '9' or >= 'a' and <= 'f');

    /// <summary>
    /// Un PIN u OTP vacío significa «no se pide». Si hay valor, debe ser UTF-8
    /// sin caracteres de control y de tamaño razonable.
    /// </summary>
    public static void ValidateSecret(byte[]? secret, string parameterName)
    {
        if (secret is null || secret.Length == 0) return;
        if (secret.Length > MaximumSecretBytes)
        {
            throw new ArgumentException("csc.gui.falta_dato", parameterName);
        }
        var remaining = secret.AsSpan();
        while (!remaining.IsEmpty)
        {
            var status = Rune.DecodeFromUtf8(remaining, out var rune, out var consumed);
            if (status != OperationStatus.Done || consumed <= 0 ||
                Rune.GetUnicodeCategory(rune) == UnicodeCategory.Control)
            {
                throw new ArgumentException("csc.gui.falta_dato", parameterName);
            }
            remaining = remaining[consumed..];
        }
    }

    /// <summary>
    /// Devuelve la clave de catálogo del error de firma remota que indica el
    /// motor («csc_otp_lote» → «csc.error.otp_lote»), o null si no lo es.
    /// </summary>
    /// <summary>
    /// Un certificado remoto pide el diálogo de PIN/OTP antes de firmar si
    /// el prestador exige alguno de los dos.
    /// </summary>
    public static bool NeedsSecrets(CertificateInfo? certificate) =>
        certificate is { Remote: true } &&
        (certificate.RemotePin || certificate.RemoteOtp);

    public static string? MessageKey(string? errorCode)
    {
        if (string.IsNullOrEmpty(errorCode) || errorCode.Length > 64 ||
            !errorCode.StartsWith("csc_", StringComparison.Ordinal))
        {
            return null;
        }
        var code = errorCode[4..];
        return code.Length > 0 && code.All(c => c is >= 'a' and <= 'z' or '_')
            ? "csc.error." + code
            : null;
    }
}

/// <summary>
/// PIN y OTP que la persona ha escrito para una firma remota. Los buffers se
/// entregan al cliente IPC, que los borra al terminar; Dispose los borra
/// también si la firma no llega a enviarse.
/// </summary>
public sealed class RemoteSigningSecrets : IDisposable
{
    public RemoteSigningSecrets(byte[]? pin, byte[]? otp)
    {
        Pin = pin is { Length: > 0 } ? pin : null;
        Otp = otp is { Length: > 0 } ? otp : null;
    }

    public byte[]? Pin { get; }
    public byte[]? Otp { get; }

    public void Dispose()
    {
        if (Pin is not null) System.Security.Cryptography.CryptographicOperations.ZeroMemory(Pin);
        if (Otp is not null) System.Security.Cryptography.CryptographicOperations.ZeroMemory(Otp);
    }
}
