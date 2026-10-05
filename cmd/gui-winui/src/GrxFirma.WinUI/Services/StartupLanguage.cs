// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Services;

// Idioma de la aplicación antes de crear la interfaz. Los textos propios de
// WinUI (mes del selector de fecha, «Cerrar navegación», nombres de los
// controles para el lector de pantalla) se cargan al construir la ventana y
// solo siguen PrimaryLanguageOverride si ya está fijado en ese momento; el
// motor confirma después el idioma por IPC.
internal static class StartupLanguage
{
    private static string SettingsPath => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
        "GrxFirma", "settings.json");

    public static void Apply()
    {
        Localizer.SetLanguage(Read());
        ApplyOverride(Localizer.Language);
    }

    public static void ApplyOverride(string? language)
    {
        try
        {
            Microsoft.Windows.Globalization.ApplicationLanguages.PrimaryLanguageOverride =
                AppCulture.Tag(language);
        }
        catch (Exception exception) when (exception is ArgumentException or
            System.Runtime.InteropServices.COMException or InvalidOperationException)
        {
            // Sin soporte en este equipo: quedan los formatos de Language.
        }
    }

    private static string? Read()
    {
        try
        {
            var info = new FileInfo(SettingsPath);
            if (!info.Exists || info.Attributes.HasFlag(FileAttributes.ReparsePoint) ||
                info.Length is < 1 or > 4 * 1024 * 1024) return null;
            using var document = JsonDocument.Parse(File.ReadAllBytes(SettingsPath));
            return document.RootElement.ValueKind == JsonValueKind.Object &&
                document.RootElement.TryGetProperty("idioma", out var value) &&
                value.ValueKind == JsonValueKind.String
                    ? value.GetString()
                    : null;
        }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException or JsonException)
        {
            return null;
        }
    }
}
