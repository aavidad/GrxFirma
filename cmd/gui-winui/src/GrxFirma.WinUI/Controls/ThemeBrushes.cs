// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.CompilerServices;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Controls;

// Los pinceles de App.xaml viven en ThemeDictionaries. Leerlos con
// Application.Current.Resources[...] devuelve la variante del tema de Windows,
// no la del tema elegido en GrxFirma (oscuro por defecto). Este ayudante toma
// el tema real del elemento (o de la raíz de la ventana) y el alto contraste.
public static class ThemeBrushes
{
    public const string Success = "AppDiagnosticSuccessBrush";
    public const string Failure = "AppDiagnosticFailureBrush";
    public const string Skipped = "AppDiagnosticSkippedBrush";
    public const string Unknown = "AppDiagnosticUnknownBrush";

    // Claves de ResourceDictionary.ThemeDictionaries.
    private const string HighContrastKey =
        nameof(Windows.UI.ViewManagement.AccessibilitySettings.HighContrast);
    private static WeakReference<FrameworkElement>? s_themeRoot;

    private sealed class BrushSet(FrameworkElement? element)
    {
        public WeakReference<FrameworkElement>? Element { get; } = element is null
            ? null : new(element);
        public Dictionary<string, SolidColorBrush> Brushes { get; } = [];
    }

    private static readonly ConditionalWeakTable<FrameworkElement, BrushSet> s_sets = new();
    private static readonly BrushSet s_default = new(null);
    private static readonly List<WeakReference<BrushSet>> s_liveSets = [new(s_default)];
    private static Windows.UI.ViewManagement.AccessibilitySettings? s_accessibility;

    public static void SetThemeRoot(FrameworkElement root)
    {
        if (s_themeRoot?.TryGetTarget(out var previous) == true)
            previous.ActualThemeChanged -= OnRootThemeChanged;
        s_themeRoot = new(root);
        root.ActualThemeChanged += OnRootThemeChanged;
        if (s_accessibility is null)
        {
            s_accessibility = new();
            s_accessibility.HighContrastChanged += (_, _) => QueueRefresh();
        }
        QueueRefresh();
    }

    private static void OnRootThemeChanged(FrameworkElement sender, object args) => QueueRefresh();

    private static void QueueRefresh()
    {
        if (s_themeRoot?.TryGetTarget(out var root) == true)
            root.DispatcherQueue.TryEnqueue(RefreshAll);
    }

    private static void RefreshAll()
    {
        for (var index = s_liveSets.Count - 1; index >= 0; index--)
        {
            if (s_liveSets[index].TryGetTarget(out var set)) Refresh(set);
            else s_liveSets.RemoveAt(index);
        }
    }

    private static void Refresh(BrushSet set)
    {
        FrameworkElement? element = null;
        set.Element?.TryGetTarget(out element);
        foreach (var (key, brush) in set.Brushes)
            if (Resolve(key, element) is SolidColorBrush source)
                brush.Color = source.Color;
    }

    public static Brush Get(string key, FrameworkElement? element = null)
    {
        var set = element is null ? s_default : s_sets.GetValue(element, owner =>
        {
            var created = new BrushSet(owner);
            s_liveSets.Add(new(created));
            owner.ActualThemeChanged += (sender, _) =>
            {
                if (s_sets.TryGetValue(sender, out var current)) Refresh(current);
            };
            owner.Loaded += (sender, _) =>
            {
                if (sender is FrameworkElement loaded && s_sets.TryGetValue(loaded, out var current))
                    Refresh(current);
            };
            return created;
        });
        var source = Resolve(key, element);
        if (source is not SolidColorBrush solid) return source;
        if (!set.Brushes.TryGetValue(key, out var brush))
        {
            brush = new SolidColorBrush(solid.Color);
            set.Brushes.Add(key, brush);
        }
        return brush;
    }

    private static Brush Resolve(string key, FrameworkElement? element)
    {
        var resources = Application.Current.Resources;
        var theme = ThemeKey(element);
        if (resources.ThemeDictionaries.TryGetValue(theme, out var dictionary) &&
            dictionary is ResourceDictionary themed &&
            themed.TryGetValue(key, out var value) &&
            value is Brush brush)
        {
            return brush;
        }
        return (Brush)resources[key];
    }

    private static string ThemeKey(FrameworkElement? element)
    {
        if (IsHighContrast()) return HighContrastKey;
        var root = element;
        if (root is null && s_themeRoot?.TryGetTarget(out var stored) == true) root = stored;
        var theme = root?.ActualTheme ?? ElementTheme.Default;
        if (theme == ElementTheme.Default)
        {
            theme = Application.Current.RequestedTheme == ApplicationTheme.Light
                ? ElementTheme.Light
                : ElementTheme.Dark;
        }
        return theme == ElementTheme.Light ? nameof(ElementTheme.Light) : nameof(ElementTheme.Dark);
    }

    private static bool IsHighContrast()
    {
        try
        {
            return new Windows.UI.ViewManagement.AccessibilitySettings().HighContrast;
        }
        catch (Exception)
        {
            return false;
        }
    }
}
