// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;

namespace GrxFirma.WinUI.Services;

internal static class SealUiCatalog
{
    private const string LogoOpacityKey = "sign.seal.logo_opacity";

    public static string LogoOpacityLabel(string? language)
        => Text(language, LogoOpacityKey);

    public static string SealOpacityHelp(string? language)
        => Text(language, "sign.seal.opacity_help");

    public static string Text(string? language, string key)
    {
        var locale = language is "ca" or "de" or "en" or "es" or "eu" or
            "fr" or "gl" or "it" or "pt" or "va" or "zh"
            ? language!
            : "es";
        return Read(locale, key) ?? Read("es", key) ?? string.Empty;
    }

    private static string? Read(string language, string key)
    {
        try
        {
            var path = Path.Combine(
                AppContext.BaseDirectory,
                "locales",
                language + ".json");
            using var stream = File.OpenRead(path);
            using var document = JsonDocument.Parse(stream);
            return document.RootElement.TryGetProperty(
                key,
                out var value) && value.ValueKind == JsonValueKind.String
                ? value.GetString()
                : null;
        }
        catch (IOException)
        {
            return null;
        }
        catch (JsonException)
        {
            return null;
        }
        catch (UnauthorizedAccessException)
        {
            return null;
        }
    }
}
