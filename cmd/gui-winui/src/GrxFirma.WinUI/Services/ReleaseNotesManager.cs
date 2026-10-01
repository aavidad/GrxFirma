// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;
using System.Text;
using System.Text.Json;

namespace GrxFirma.WinUI.Services;

internal sealed class ReleaseNotesManager
{
    private readonly string _settingsPath;
    private readonly string _source = string.Empty;
    public string InstalledVersion { get; } = string.Empty;
    public string Pending { get; private set; } = string.Empty;

    public ReleaseNotesManager()
    {
        var versionPath = Path.Combine(AppContext.BaseDirectory, "VERSION.txt");
        try
        {
            var info = new FileInfo(versionPath);
            if (info.Exists && !info.Attributes.HasFlag(FileAttributes.ReparsePoint) &&
                info.Length is > 0 and <= 64)
            {
                var value = File.ReadAllText(versionPath).Trim();
                if (ReleaseNotesSections.TryVersion(value, out _))
                {
                    InstalledVersion = value;
                }
            }
        }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }

        var notesPath = Path.Combine(AppContext.BaseDirectory, "help", "NOVEDADES.md");
        try
        {
            var info = new FileInfo(notesPath);
            var directory = new DirectoryInfo(Path.GetDirectoryName(notesPath)!);
            if (directory.Exists &&
                !directory.Attributes.HasFlag(FileAttributes.ReparsePoint) &&
                info.Exists && !info.Attributes.HasFlag(FileAttributes.ReparsePoint) &&
                info.Length is > 0 and <= 64 * 1024)
            {
                _source = File.ReadAllText(notesPath, new UTF8Encoding(false, true));
            }
        }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
        catch (DecoderFallbackException) { }

        var userDirectory = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
            "GrxFirma");
        _settingsPath = Path.Combine(userDirectory, "ui-settings.json");
        var lastSeen = ReadLastSeen();
        if (lastSeen is null)
        {
            // Primera instalación: establecer la referencia sin interrumpir.
            SaveLastSeen();
            Pending = string.Empty;
        }
        else
        {
            Pending = lastSeen == InstalledVersion
                ? string.Empty
                : ReleaseNotesSections.Select(_source, InstalledVersion, lastSeen);
        }
    }

    public string History => ReleaseNotesSections.Select(_source, InstalledVersion);

    public void Acknowledge()
    {
        SaveLastSeen();
        Pending = string.Empty;
    }

    private string? ReadLastSeen()
    {
        try
        {
            var info = new FileInfo(_settingsPath);
            if (!info.Exists || info.Attributes.HasFlag(FileAttributes.ReparsePoint) ||
                info.Length > 4096)
            {
                return null;
            }
            using var document = JsonDocument.Parse(File.ReadAllText(_settingsPath));
            var value = document.RootElement.GetProperty("lastSeenVersion").GetString();
            return ReleaseNotesSections.TryVersion(value, out _) ? value : null;
        }
        catch (IOException) { return null; }
        catch (UnauthorizedAccessException) { return null; }
        catch (JsonException) { return null; }
        catch (KeyNotFoundException) { return null; }
        catch (InvalidOperationException) { return null; }
    }

    private void SaveLastSeen()
    {
        if (InstalledVersion.Length == 0)
        {
            return;
        }
        var temporary = _settingsPath + "." + Guid.NewGuid().ToString("N") + ".tmp";
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(_settingsPath)!);
            if (new DirectoryInfo(Path.GetDirectoryName(_settingsPath)!)
                .Attributes.HasFlag(FileAttributes.ReparsePoint) ||
                (File.Exists(_settingsPath) && new FileInfo(_settingsPath)
                    .Attributes.HasFlag(FileAttributes.ReparsePoint)))
                return;
            File.WriteAllText(temporary,
                JsonSerializer.Serialize(new { lastSeenVersion = InstalledVersion }));
            File.Move(temporary, _settingsPath, true);
        }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
        finally
        {
            try { File.Delete(temporary); }
            catch (IOException) { }
            catch (UnauthorizedAccessException) { }
        }
    }
}
