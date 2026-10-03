// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;

namespace GrxFirma.WinUI.Services;

internal static class UpdatePreferenceStore
{
    private static string DirectoryPath => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "GrxFirma");
    private static string MirrorPath => Path.Combine(DirectoryPath, "update-preference.json");
    private static string EnginePath => Path.Combine(DirectoryPath, "settings.json");
    private static string LastAttemptPath => Path.Combine(DirectoryPath, "update-last-attempt.json");

    // El documento del motor es la fuente de verdad, incluso si no puede arrancar.
    public static bool Read() => ReadFile(EnginePath, 4 * 1024 * 1024)
        ?? ReadFile(MirrorPath, 1024) ?? true;

    private static bool? ReadFile(string path, long maximumBytes)
    {
        try
        {
            var info = new FileInfo(path);
            if (!info.Exists || info.Attributes.HasFlag(FileAttributes.ReparsePoint) ||
                info.Length is < 1 || info.Length > maximumBytes) return null;
            using var document = JsonDocument.Parse(File.ReadAllBytes(path));
            if (document.RootElement.ValueKind != JsonValueKind.Object) return null;
            if (!document.RootElement.TryGetProperty("checkForUpdates", out var value)) return null;
            return value.ValueKind switch
            {
                JsonValueKind.True => true,
                JsonValueKind.False => false,
                _ => null,
            };
        }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException or JsonException)
        {
            return null;
        }
    }

    public static bool TryMarkAttempt(DateTimeOffset now)
    {
        if (!GrxFirma.WinUI.Core.Operations.UpdateNoticeSchedule.IsDueSince(
                ReadLastAttempt(), now)) return false;
        try
        {
            Directory.CreateDirectory(DirectoryPath);
            var temporary = LastAttemptPath + "." + Guid.NewGuid().ToString("N") + ".tmp";
            File.WriteAllText(temporary, JsonSerializer.Serialize(new { lastAttempt = now }));
            File.Move(temporary, LastAttemptPath, true);
            return true;
        }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException)
        {
            return false;
        }
    }

    private static DateTimeOffset? ReadLastAttempt()
    {
        try
        {
            var info = new FileInfo(LastAttemptPath);
            if (!info.Exists || info.Length is < 1 or > 1024 ||
                info.Attributes.HasFlag(FileAttributes.ReparsePoint)) return null;
            using var document = JsonDocument.Parse(File.ReadAllBytes(LastAttemptPath));
            if (document.RootElement.TryGetProperty("lastAttempt", out var value) &&
                value.ValueKind == JsonValueKind.String &&
                DateTimeOffset.TryParse(value.GetString(), out var parsed))
                return parsed;
        }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException or JsonException) { }
        return null;
    }

    public static void Write(bool enabled)
    {
        try
        {
            Directory.CreateDirectory(DirectoryPath);
            var temporary = MirrorPath + "." + Guid.NewGuid().ToString("N") + ".tmp";
            File.WriteAllText(temporary, JsonSerializer.Serialize(new { checkForUpdates = enabled }));
            File.Move(temporary, MirrorPath, true);
        }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException) { }
    }
}
