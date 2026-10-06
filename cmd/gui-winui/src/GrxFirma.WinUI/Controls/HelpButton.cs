// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;

namespace GrxFirma.WinUI.Controls;

/// <summary>
/// Botón «?» que explica una opción técnica. Se abre al pulsarlo (ratón,
/// táctil, Intro o Espacio) y se cierra con Escape; nunca depende de pasar el
/// ratón. Todos los textos salen del catálogo: <see cref="HelpKey"/> nombra
/// una o varias claves <c>ayuda.*</c> separadas por espacios y
/// <see cref="Topic"/> es la etiqueta visible del control al que ayuda, que
/// forma el nombre accesible «Ayuda sobre {0}» (<c>ayuda.boton_nombre</c>).
/// </summary>
public sealed class HelpButton : Button
{
    private const string HelpGlyph = "\uE9CE"; // Unknown (círculo con interrogación) de Segoe Fluent Icons
    private const string AccessibleNameKey = "ayuda.boton_nombre";

    public static readonly DependencyProperty TopicProperty = DependencyProperty.Register(
        nameof(Topic), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    public static readonly DependencyProperty HelpKeyProperty = DependencyProperty.Register(
        nameof(HelpKey), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    public static readonly DependencyProperty DetailKeyProperty = DependencyProperty.Register(
        nameof(DetailKey), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    public static readonly DependencyProperty HeadingKeyProperty = DependencyProperty.Register(
        nameof(HeadingKey), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    private readonly StackPanel _body = new() { MaxWidth = 360, Spacing = 8 };
    private bool _ownsName;
    private bool _ownsHelpText;
    private bool _ownsToolTip;

    public HelpButton()
    {
        MinWidth = 40;
        MinHeight = 40;
        Padding = new Thickness(0);
        VerticalAlignment = VerticalAlignment.Bottom;
        HorizontalAlignment = HorizontalAlignment.Left;
        Content = new FontIcon { FontSize = 16, Glyph = HelpGlyph };
        var flyout = new Flyout
        {
            Content = _body,
            Placement = FlyoutPlacementMode.BottomEdgeAlignedLeft,
            ShouldConstrainToRootBounds = true,
        };
        flyout.Opening += (_, _) => Refresh();
        Flyout = flyout;
        Loaded += (_, _) => Refresh();
    }

    /// <summary>Etiqueta visible del control (texto español del catálogo o clave).</summary>
    public string Topic
    {
        get => (string)GetValue(TopicProperty);
        set => SetValue(TopicProperty, value);
    }

    /// <summary>Clave o claves <c>ayuda.*</c> separadas por espacios.</summary>
    public string HelpKey
    {
        get => (string)GetValue(HelpKeyProperty);
        set => SetValue(HelpKeyProperty, value);
    }

    /// <summary>Clave adicional, por ejemplo la de la opción elegida en un selector.</summary>
    public string DetailKey
    {
        get => (string)GetValue(DetailKeyProperty);
        set => SetValue(DetailKeyProperty, value);
    }

    /// <summary>Título opcional; si falta, el título es la etiqueta del control.</summary>
    public string HeadingKey
    {
        get => (string)GetValue(HeadingKeyProperty);
        set => SetValue(HeadingKeyProperty, value);
    }

    private static void OnTextChanged(DependencyObject sender, DependencyPropertyChangedEventArgs args)
    {
        if (sender is HelpButton { IsLoaded: true } button) button.Refresh();
    }

    private static IEnumerable<string> Keys(string? keys) =>
        (keys ?? string.Empty).Split(' ', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);

    private string Paragraphs() =>
        string.Join(" ", Keys(HelpKey).Concat(Keys(DetailKey)).Select(Localizer.Text));

    // Se recalcula al cargar y al abrir: el idioma puede haber cambiado y un
    // selector puede haber cambiado de opción. Un nombre, ayuda o descripción
    // emergente que la página fije en XAML se respeta.
    private void Refresh()
    {
        var topic = Localizer.VisibleText(Topic ?? string.Empty);
        var name = Localizer.Format(AccessibleNameKey, topic);
        if (_ownsName || ReadLocalValue(AutomationProperties.NameProperty) == DependencyProperty.UnsetValue)
        {
            _ownsName = true;
            AutomationProperties.SetName(this, name);
        }
        if (_ownsToolTip || ReadLocalValue(ToolTipService.ToolTipProperty) == DependencyProperty.UnsetValue)
        {
            _ownsToolTip = true;
            ToolTipService.SetToolTip(this, name);
        }
        var text = Paragraphs();
        if (_ownsHelpText || ReadLocalValue(AutomationProperties.HelpTextProperty) == DependencyProperty.UnsetValue)
        {
            _ownsHelpText = true;
            AutomationProperties.SetHelpText(this, text);
        }

        _body.Children.Clear();
        var heading = string.IsNullOrWhiteSpace(HeadingKey) ? topic : Localizer.Text(HeadingKey);
        var title = new TextBlock
        {
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            Text = heading,
            TextWrapping = TextWrapping.Wrap,
        };
        AutomationProperties.SetHeadingLevel(title, AutomationHeadingLevel.Level3);
        _body.Children.Add(title);
        foreach (var key in Keys(HelpKey).Concat(Keys(DetailKey)))
        {
            _body.Children.Add(new TextBlock
            {
                IsTextSelectionEnabled = true,
                Text = Localizer.Text(key),
                TextWrapping = TextWrapping.Wrap,
            });
        }
    }
}
