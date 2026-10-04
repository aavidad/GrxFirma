// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Reflection;
using System.Runtime.CompilerServices;
using System.Text.RegularExpressions;
using GrxFirma.WinUI.Core.Localization;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Services;

internal static class Localizer
{
    private static readonly CatalogLocalizer Catalog = new(
        Path.Combine(AppContext.BaseDirectory, "locales"));
    private static readonly Regex PlaceholderPattern = new(
        @"\{([a-z]+)\}", RegexOptions.CultureInvariant);
    private static readonly ConditionalWeakTable<DependencyObject, ElementState> States = new();
    private static readonly Dictionary<Type, DependencyProperty[]> PropertyCache = [];
    private static readonly string[] VisibleProperties =
    [
        "Text", "Content", "Header", "PlaceholderText", "Title", "Message",
        "Description", "PrimaryButtonText", "SecondaryButtonText", "CloseButtonText",
        "OnContent", "OffContent",
    ];

    private sealed class ElementState
    {
        public Dictionary<DependencyProperty, string> Original { get; } = [];
        public HashSet<DependencyProperty> Observed { get; } = [];
        public bool Applying { get; set; }
    }

    public static string Language => Catalog.Language;
    public static string Text(string key) => Catalog.TranslateVisibleText(key);
    public static string Text(string? language, string key) => Catalog.Text(language, key);

    public static string Fill(string key, params (string Name, string Value)[] values)
    {
        var replacements = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var (name, value) in values)
            replacements[name] = value;
        return PlaceholderPattern.Replace(Text(key), match =>
            replacements.TryGetValue(match.Groups[1].Value, out var replacement)
                ? replacement
                : match.Value);
    }

    public static bool SetLanguage(string? language) => Catalog.SetLanguage(language);

    // Se registra una vez por ventana. LayoutUpdated incorpora controles que
    // se crean después (elementos de ComboBox, resultados y avisos).
    public static void Attach(FrameworkElement root)
    {
        ArgumentNullException.ThrowIfNull(root);
        root.LayoutUpdated += (_, _) => Apply(root);
        Apply(root);
    }

    public static void Apply(DependencyObject root)
    {
        ArgumentNullException.ThrowIfNull(root);
        Scan(root);
    }

    public static async Task<ContentDialogResult> ShowAsync(ContentDialog dialog)
    {
        Apply(dialog);
        return await dialog.ShowAsync();
    }

    private static void Scan(DependencyObject element)
    {
        foreach (var property in Properties(element.GetType())) Observe(element, property);
        Observe(element, AutomationProperties.NameProperty);
        Observe(element, AutomationProperties.HelpTextProperty);
        Observe(element, AutomationProperties.FullDescriptionProperty);
        Observe(element, ToolTipService.ToolTipProperty);

        // El contenido de cuadros de diálogo puede no haberse agregado aún
        // al árbol visual cuando se llama a ShowAsync.
        if (element is ContentDialog { Content: DependencyObject content })
            Scan(content);
        if (element is ContentControl { Content: DependencyObject nested })
            Scan(nested);
        if (element is Border { Child: DependencyObject child })
            Scan(child);
        if (element is Panel panel)
            foreach (var child in panel.Children) Scan(child);
        var count = VisualTreeHelper.GetChildrenCount(element);
        for (var i = 0; i < count; i++)
            Scan(VisualTreeHelper.GetChild(element, i));
    }

    private static DependencyProperty[] Properties(Type type)
    {
        if (PropertyCache.TryGetValue(type, out var cached)) return cached;
        var properties = new List<DependencyProperty>();
        foreach (var name in VisibleProperties)
        {
            // TextBox.Text y PasswordBox.Password contienen datos del usuario.
            if (name == "Text" && !typeof(TextBlock).IsAssignableFrom(type) &&
                type.Name != "Run") continue;
            var field = type.GetField(name + "Property",
                BindingFlags.Public | BindingFlags.Static | BindingFlags.FlattenHierarchy);
            if (field?.GetValue(null) is DependencyProperty property)
                properties.Add(property);
        }
        cached = [.. properties];
        PropertyCache[type] = cached;
        return cached;
    }

    private static void Observe(DependencyObject element, DependencyProperty property)
    {
        var state = States.GetValue(element, static _ => new ElementState());
        if (state.Observed.Add(property))
        {
            element.RegisterPropertyChangedCallback(property, (sender, changed) =>
            {
                var currentState = States.GetValue(sender, static _ => new ElementState());
                if (currentState.Applying) return;
                if (sender.GetValue(changed) is string current &&
                    !string.IsNullOrWhiteSpace(current))
                {
                    currentState.Original[changed] = current;
                    Translate(sender, changed, currentState);
                }
            });
        }
        if (!state.Original.ContainsKey(property) &&
            element.GetValue(property) is string initial &&
            !string.IsNullOrWhiteSpace(initial))
            state.Original[property] = initial;
        if (!state.Original.ContainsKey(property)) return;
        Translate(element, property, state);
    }

    private static void Translate(
        DependencyObject element, DependencyProperty property, ElementState state)
    {
        var original = state.Original[property];
        var translated = Catalog.TranslateVisibleText(original);
        if (element.GetValue(property) is string current && current == translated)
            return;
        state.Applying = true;
        try { element.SetValue(property, translated); }
        finally { state.Applying = false; }
    }
}
