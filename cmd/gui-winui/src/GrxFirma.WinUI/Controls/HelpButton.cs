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
using Microsoft.UI.Xaml.Documents;

namespace GrxFirma.WinUI.Controls;

/// <summary>
/// Botón «?» que explica una opción técnica. Se abre al pulsarlo (ratón,
/// táctil, Intro o Espacio) y se cierra con Escape; nunca depende de pasar el
/// ratón. Todos los textos salen del catálogo: <see cref="HelpKey"/> nombra
/// una o varias claves <c>ayuda.*</c> separadas por espacios y
/// <see cref="Topic"/> es la etiqueta visible del control al que ayuda, que
/// forma el nombre accesible «Ayuda sobre {0}» (<c>ayuda.boton_nombre</c>).
/// En un selector, <see cref="Options"/> enumera todas sus opciones, cada una
/// con su nombre en negrita y su frase breve, sin depender de la elegida.
/// Si una clave tiene en el catálogo su ampliación <c>&lt;clave&gt;.mas</c>,
/// al final de su frase aparece un «+» que la despliega.
/// </summary>
public sealed class HelpButton : Button
{
    private const string HelpGlyph = "\uE9CE"; // Unknown (círculo con interrogación) de Segoe Fluent Icons
    private const string AccessibleNameKey = "ayuda.boton_nombre";
    private const string MoreNameKey = "ayuda.mas_nombre";
    private const string OptionKey = "ayuda.opcion";
    private const string MoreSuffix = ".mas";

    public static readonly DependencyProperty TopicProperty = DependencyProperty.Register(
        nameof(Topic), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    public static readonly DependencyProperty HelpKeyProperty = DependencyProperty.Register(
        nameof(HelpKey), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    public static readonly DependencyProperty OptionsProperty = DependencyProperty.Register(
        nameof(Options), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    public static readonly DependencyProperty HeadingKeyProperty = DependencyProperty.Register(
        nameof(HeadingKey), typeof(string), typeof(HelpButton),
        new PropertyMetadata(string.Empty, OnTextChanged));

    private readonly StackPanel _body = new() { MaxWidth = 360, Spacing = 8 };
    // Con las ampliaciones desplegadas el texto puede no caber: se desplaza.
    private readonly ScrollViewer _scroll = new()
    {
        MaxHeight = 440,
        VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
        HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
    };
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
        // Solo el círculo con la interrogación: sin fondo ni borde de botón.
        // El área táctil sigue siendo de 40 × 40 y al pasar el ratón o con el
        // foco se ve un círculo suave del tema.
        Background = new Microsoft.UI.Xaml.Media.SolidColorBrush(Microsoft.UI.Colors.Transparent);
        BorderThickness = new Thickness(0);
        CornerRadius = new CornerRadius(20);
        Content = new FontIcon { FontSize = 20, Glyph = HelpGlyph };
        _scroll.Content = _body;
        var flyout = new Flyout
        {
            Content = _scroll,
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

    /// <summary>
    /// Todas las opciones de un selector, separadas por espacios, en la forma
    /// <c>nombre=clave</c>: el nombre es la etiqueta de la opción (clave o
    /// texto del catálogo) y la clave, su frase breve <c>ayuda.*</c>.
    /// </summary>
    public string Options
    {
        get => (string)GetValue(OptionsProperty);
        set => SetValue(OptionsProperty, value);
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

    private IEnumerable<(string? Name, string Key)> Entries()
    {
        foreach (var key in Keys(HelpKey)) yield return (null, key);
        foreach (var option in Keys(Options))
        {
            var separator = option.IndexOf('=');
            if (separator <= 0 || separator == option.Length - 1) continue;
            yield return (Localizer.Text(option[..separator]), option[(separator + 1)..]);
        }
    }

    // Texto que lee el lector de pantalla: lo mismo que se ve, con todas las
    // opciones (las ampliaciones se leen al desplegarlas).
    private string Paragraphs() =>
        string.Join(" ", Entries().Select(entry => entry.Name is null
            ? Localizer.Text(entry.Key)
            : Localizer.Format(OptionKey, entry.Name, Localizer.Text(entry.Key))));

    // Se recalcula al cargar y al abrir: el idioma puede haber cambiado. Un
    // nombre, ayuda o descripción emergente que la página fije en XAML se respeta.
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
        foreach (var (optionName, key) in Entries())
            _body.Children.Add(Entry(optionName, key, topic));
    }

    // Una opción: «Nombre: frase» con el nombre en negrita y, si el catálogo
    // tiene su ampliación, un «+» que la despliega debajo.
    private static FrameworkElement Entry(string? optionName, string key, string topic)
    {
        var sentence = new TextBlock { IsTextSelectionEnabled = true, TextWrapping = TextWrapping.Wrap };
        if (optionName is null)
        {
            sentence.Text = Localizer.Text(key);
        }
        else
        {
            foreach (var part in OptionParts.Split(Localizer.Text(OptionKey)))
            {
                if (part.Length == 0) continue;
                sentence.Inlines.Add(part switch
                {
                    "{0}" => new Run { Text = optionName, FontWeight = Microsoft.UI.Text.FontWeights.Bold },
                    "{1}" => new Run { Text = Localizer.Text(key) },
                    _ => new Run { Text = part },
                });
            }
        }

        var moreKey = key + MoreSuffix;
        if (!Localizer.Has(moreKey)) return sentence;

        var grid = new Grid { ColumnSpacing = 4, RowSpacing = 4 };
        grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        grid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.Children.Add(sentence);

        var detail = new TextBlock
        {
            IsTextSelectionEnabled = true,
            Text = Localizer.Text(moreKey),
            TextWrapping = TextWrapping.Wrap,
            Visibility = Visibility.Collapsed,
        };
        Grid.SetRow(detail, 1);
        Grid.SetColumnSpan(detail, 2);
        grid.Children.Add(detail);

        var more = new MoreInfoButton();
        AutomationProperties.SetName(more, Localizer.Format(MoreNameKey, optionName ?? topic));
        more.ExpandedChanged += (_, _) =>
            detail.Visibility = more.IsExpanded ? Visibility.Visible : Visibility.Collapsed;
        Grid.SetColumn(more, 1);
        grid.Children.Add(more);
        return grid;
    }

    private static readonly System.Text.RegularExpressions.Regex OptionParts =
        new(@"(\{[01]\})", System.Text.RegularExpressions.RegexOptions.CultureInvariant);
}
