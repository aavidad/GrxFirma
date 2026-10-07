// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

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

    public static void SetThemeRoot(FrameworkElement root) =>
        s_themeRoot = new WeakReference<FrameworkElement>(root);

    public static Brush Get(string key, FrameworkElement? element = null)
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
