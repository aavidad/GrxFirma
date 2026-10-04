// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.Json;
using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Localization;

/// <summary>Lee los mismos catálogos que Qt, sin mantener una segunda copia.</summary>
public sealed class CatalogLocalizer
{
    private static readonly HashSet<string> Supported =
        ["ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"];

    // Core and WinUI use one language selection and the same packaged catalogs.
    public static CatalogLocalizer Shared { get; } = new(
        Path.Combine(AppContext.BaseDirectory, "locales"));

    private readonly string _directory;
    private readonly object _sync = new();
    private readonly Dictionary<string, IReadOnlyDictionary<string, string>> _catalogs = [];
    private readonly Dictionary<string, string> _spanishValues = new(StringComparer.Ordinal);
    private readonly List<(string Key, Regex Pattern, int ArgumentCount)> _visibleTemplates = [];
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
        => TranslateVisibleText(source, translateCapturedSpanish: false);

    // Los errores compuestos de Facturae insertan nombres de campos españoles.
    // Traducir esas capturas solo allí evita alterar nombres de archivo y datos.
    public string TranslateVisibleText(string source, bool translateCapturedSpanish)
    {
        if (string.IsNullOrWhiteSpace(source)) return source;
        lock (_sync) return TranslateVisibleTextCore(source, translateCapturedSpanish);
    }

    private string TranslateVisibleTextCore(string source, bool translateCapturedSpanish)
    {
        if (Read("es").ContainsKey(source)) return Text(source);
        if (_spanishValues.TryGetValue(source, out var key)) return Text(key);
        if (source.Length > 2048) return source;
        foreach (var template in _visibleTemplates)
        {
            Match match;
            try { match = template.Pattern.Match(source); }
            catch (RegexMatchTimeoutException) { continue; }
            if (!match.Success) continue;
            var arguments = new object[template.ArgumentCount];
            for (var index = 0; index < arguments.Length; index++)
            {
                var captured = match.Groups[$"value{index}"].Value;
                // Un campo puede contener otra plantilla (p. ej. «El NIF del
                // receptor»). Cada captura es más corta que la frase origen.
                arguments[index] = translateCapturedSpanish
                    ? TranslateVisibleTextCore(captured, true)
                    : captured;
            }
            try
            {
                return string.Format(CultureInfo.CurrentCulture,
                    Text(template.Key), arguments);
            }
            catch (FormatException) { return source; }
        }
        return source;
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
            {
                _spanishValues.TryAdd(entry.Value, entry.Key);
                // Los mensajes compuestos de los ViewModels ya contienen los
                // valores insertados cuando llegan a las propiedades de WinUI.
                if (entry.Key.Contains("{0}", StringComparison.Ordinal))
                {
                    var pattern = BuildVisibleTemplate(entry.Key);
                    if (pattern is not null) _visibleTemplates.Add(pattern.Value);
                }
            }
            _visibleTemplates.Sort((left, right) =>
                right.Key.Length.CompareTo(left.Key.Length));
        }
        return catalog;
    }

    private static (string Key, Regex Pattern, int ArgumentCount)? BuildVisibleTemplate(
        string key)
    {
        var placeholders = Regex.Matches(key, @"\{(\d+)\}");
        if (placeholders.Count == 0) return null;
        // Las plantillas genéricas «{0}: {1}» se usan con Format de forma
        // explícita. Aplicarlas al árbol visual también alteraría URLs y datos.
        var fixedText = Regex.Replace(key, @"\{\d+\}", string.Empty);
        if (Regex.Matches(fixedText, @"\p{L}").Count < 5) return null;
        var pattern = new System.Text.StringBuilder("^");
        var position = 0;
        var maximum = -1;
        var seen = new HashSet<int>();
        foreach (Match placeholder in placeholders)
        {
            var index = int.Parse(placeholder.Groups[1].Value,
                CultureInfo.InvariantCulture);
            if (index > 9) return null;
            maximum = Math.Max(maximum, index);
            pattern.Append(Regex.Escape(key[position..placeholder.Index]));
            pattern.Append(seen.Add(index)
                ? $"(?<value{index}>.*?)"
                : $"\\k<value{index}>");
            position = placeholder.Index + placeholder.Length;
        }
        pattern.Append(Regex.Escape(key[position..])).Append('$');
        return (key, new Regex(pattern.ToString(), RegexOptions.Singleline |
            RegexOptions.CultureInvariant, TimeSpan.FromMilliseconds(50)), maximum + 1);
    }
}
