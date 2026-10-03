// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.Json;

namespace GrxFirma.WinUI.Core.Localization;

/// <summary>Lee los mismos catálogos que Qt, sin mantener una segunda copia.</summary>
public sealed class CatalogLocalizer
{
    private static readonly HashSet<string> Supported =
        ["ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"];

    private readonly string _directory;
    private readonly object _sync = new();
    private readonly Dictionary<string, IReadOnlyDictionary<string, string>> _catalogs = [];
    private readonly Dictionary<string, string> _spanishValues = new(StringComparer.Ordinal);
    private string _language;

    public CatalogLocalizer(string directory, string? language = null)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(directory);
        _directory = directory;
        _language = Normalize(language ?? CultureInfo.CurrentUICulture.Name);
    }

    public string Language { get { lock (_sync) return _language; } }

    public static string Normalize(string? language)
    {
        var code = language?.Split('-', '_')[0].ToLowerInvariant();
        return code is not null && Supported.Contains(code) ? code : "es";
    }

    public bool SetLanguage(string? language)
    {
        var normalized = Normalize(string.IsNullOrWhiteSpace(language)
            ? CultureInfo.CurrentUICulture.Name
            : language);
        lock (_sync)
        {
            if (_language == normalized) return false;
            _language = normalized;
            return true;
        }
    }

    public string Text(string key)
    {
        lock (_sync) return Text(_language, key);
    }

    public string Text(string? language, string key)
    {
        ArgumentNullException.ThrowIfNull(key);
        lock (_sync)
        {
            var locale = Normalize(language);
            if (Read(locale).TryGetValue(key, out var translated) &&
                !string.IsNullOrEmpty(translated)) return translated;
            return Read("es").TryGetValue(key, out var spanish) ? spanish : key;
        }
    }

    // El XAML heredado usa la frase española como valor. Su búsqueda se hace
    // por clave o por valor español para aprovechar también las claves Qt.
    public string TranslateVisibleText(string source)
    {
        if (string.IsNullOrWhiteSpace(source)) return source;
        lock (_sync)
        {
            if (Read("es").ContainsKey(source)) return Text(source);
            if (_spanishValues.TryGetValue(source, out var key)) return Text(key);
            return source;
        }
    }

    private IReadOnlyDictionary<string, string> Read(string language)
    {
        if (_catalogs.TryGetValue(language, out var catalog)) return catalog;
        try
        {
            using var stream = File.OpenRead(Path.Combine(_directory, language + ".json"));
            catalog = JsonSerializer.Deserialize<Dictionary<string, string>>(stream)
                ?? new Dictionary<string, string>();
        }
        catch (IOException) { catalog = new Dictionary<string, string>(); }
        catch (UnauthorizedAccessException) { catalog = new Dictionary<string, string>(); }
        catch (JsonException) { catalog = new Dictionary<string, string>(); }
        _catalogs[language] = catalog;
        if (language == "es")
        {
            foreach (var entry in catalog)
                _spanishValues.TryAdd(entry.Value, entry.Key);
        }
        return catalog;
    }
}
