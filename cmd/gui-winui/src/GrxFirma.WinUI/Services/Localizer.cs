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
    private static readonly CatalogLocalizer Catalog = CatalogLocalizer.Shared;
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
        public Dictionary<DependencyProperty, (string Source, string Language,
            string Translation)> Applied { get; } = [];
        public bool Applying { get; set; }
    }

    public static string Language => Catalog.Language;
    public static string Text(string key) => Catalog.TranslateVisibleText(key);
    public static string Text(string? language, string key) => Catalog.Text(language, key);
    public static string VisibleText(string source) => Catalog.TranslateVisibleText(source);
    public static string Format(string key, params object?[] arguments) =>
        string.Format(System.Globalization.CultureInfo.CurrentCulture,
            Catalog.Text(key), arguments);

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

    private static readonly TimeSpan RescanInterval = TimeSpan.FromMilliseconds(300);

    // Se registra una vez por ventana. LayoutUpdated incorpora controles que
    // se crean después (elementos de ComboBox, resultados y avisos), pero se
    // dispara continuamente y cada traducción provoca otra maquetación: los
    // avisos se agrupan en un único recorrido diferido, como mucho cada 300 ms.
    // Recorrer el árbol en cada LayoutUpdated dejaba la aplicación consumiendo
    // CPU y memoria sin parar.
    public static void Attach(FrameworkElement root)
    {
        ArgumentNullException.ThrowIfNull(root);
        var pending = false;
        var last = DateTimeOffset.MinValue;
        var queue = root.DispatcherQueue;
        root.LayoutUpdated += (_, _) =>
        {
            if (pending || queue is null) return;
            pending = true;
            var wait = last + RescanInterval - DateTimeOffset.UtcNow;
            async void Run()
            {
                try
                {
                    if (wait > TimeSpan.Zero) await Task.Delay(wait);
                    Apply(root);
                }
                finally
                {
                    last = DateTimeOffset.UtcNow;
                    pending = false;
                }
            }
            queue.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, Run);
        };
        Apply(root);
        last = DateTimeOffset.UtcNow;
    }

    public static void Apply(DependencyObject root)
    {
        ArgumentNullException.ThrowIfNull(root);
        Scan(root, new HashSet<DependencyObject>(ReferenceEqualityComparer.Instance));
    }

    public static async Task<ContentDialogResult> ShowAsync(ContentDialog dialog)
    {
        Apply(dialog);
        return await dialog.ShowAsync();
    }

    // Cada pasada visita cada elemento una sola vez: el contenido de paneles y
    // controles también aparece en el árbol visual, y recorrer ambos duplicaba
    // el trabajo en cada nivel de anidamiento.
    private static void Scan(DependencyObject element, HashSet<DependencyObject> visited)
    {
        if (!visited.Add(element)) return;
        foreach (var property in Properties(element.GetType())) Observe(element, property);
        Observe(element, AutomationProperties.NameProperty);
        Observe(element, AutomationProperties.HelpTextProperty);
        Observe(element, AutomationProperties.FullDescriptionProperty);
        Observe(element, ToolTipService.ToolTipProperty);

        // El contenido de cuadros de diálogo puede no haberse agregado aún
        // al árbol visual cuando se llama a ShowAsync.
        if (element is ContentDialog { Content: DependencyObject content })
            Scan(content, visited);
        if (element is ContentControl { Content: DependencyObject nested })
            Scan(nested, visited);
        if (element is Border { Child: DependencyObject borderChild })
            Scan(borderChild, visited);
        if (element is Panel panel)
            foreach (var panelChild in panel.Children) Scan(panelChild, visited);
        var count = VisualTreeHelper.GetChildrenCount(element);
        for (var i = 0; i < count; i++)
            Scan(VisualTreeHelper.GetChild(element, i), visited);
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
            // En WinUI 3 los identificadores (TextBlock.TextProperty,
            // ContentControl.ContentProperty…) son propiedades estáticas, no
            // campos: buscar solo campos dejaba sin traducir el texto del XAML.
            const BindingFlags flags =
                BindingFlags.Public | BindingFlags.Static | BindingFlags.FlattenHierarchy;
            var identifier =
                type.GetProperty(name + "Property", flags)?.GetValue(null) ??
                type.GetField(name + "Property", flags)?.GetValue(null);
            if (identifier is DependencyProperty property)
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
        var language = Catalog.Language;
        if (state.Applied.TryGetValue(property, out var applied) &&
            applied.Source == original && applied.Language == language &&
            element.GetValue(property) is string displayed &&
            displayed == applied.Translation) return;
        var translated = Catalog.TranslateVisibleText(original);
        state.Applied[property] = (original, language, translated);
        if (element.GetValue(property) is string current && current == translated)
            return;
        state.Applying = true;
        try { element.SetValue(property, translated); }
        finally { state.Applying = false; }
    }
}
