// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>
/// Material necesario para crear o rotar una credencial de proxy. Password
/// permanece binario para que System.Text.Json lo codifique como Base64 sin
/// crear una contraseña administrada como string.
/// </summary>
public sealed record ProxySecretStoreParameters
{
    [JsonPropertyName("realm")]
    public required string Realm { get; init; }

    [JsonPropertyName("username")]
    public required string Username { get; init; }

    [JsonPropertyName("password")]
    public required byte[] Password { get; init; }
}

public sealed record ProxySecretDeleteParameters;

public sealed record ProxySecretMutationResult
{
    private string _realm = string.Empty;
    private string _username = string.Empty;

    [JsonPropertyName("configured")]
    public bool Configured { get; init; }

    [JsonPropertyName("realm")]
    public string Realm
    {
        get => _realm;
        init => _realm = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("username")]
    public string Username
    {
        get => _username;
        init => _username = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("rotated")]
    public bool Rotated { get; init; }
}
