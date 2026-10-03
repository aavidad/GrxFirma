// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class RestServerLaunch
{
    public const int Port = 63118;
    public const string Address = "127.0.0.1:63118";
    public const string ConsoleUrl = "https://127.0.0.1:63118/";
    public const string TokenEnvironmentName = "GRXFIRMA_REST_TOKEN";

    public static IReadOnlyList<string> Arguments { get; } =
        ["--rest", "--rest-addr", Address];

    public static string CreateToken() =>
        Convert.ToHexString(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));

    public static bool IsUsableToken(string? token) =>
        token is { Length: >= 32 and <= 128 } &&
        token.All(static character =>
            char.IsAsciiLetterOrDigit(character) || character is '-' or '_');
}
