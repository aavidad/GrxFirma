// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using Windows.Foundation;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

/// <summary>
/// Coloca un control y su botón «?» en la misma fila. El botón va pegado al
/// final del control (no al borde de la página) y el control conserva el
/// ancho disponible menos el del botón, de modo que sus textos siguen
/// ajustándose en ventanas estrechas. Si el control se estira
/// (<c>HorizontalAlignment="Stretch"</c>), ocupa todo el ancho restante.
/// La posición vertical del botón sigue su <c>VerticalAlignment</c>: abajo
/// para alinearlo con la caja de un campo con encabezado, centrado junto a
/// una casilla.
/// </summary>
public sealed class HelpRow : Panel
{
    public double Spacing { get; set; } = 4;

    private (UIElement? Main, UIElement? Help) Parts()
    {
        UIElement? main = null, help = null;
        foreach (var child in Children)
        {
            if (child is HelpButton && help is null) help = child;
            else main ??= child;
        }
        return (main, help);
    }

    protected override Size MeasureOverride(Size availableSize)
    {
        var (main, help) = Parts();
        double helpWidth = 0, helpHeight = 0;
        if (help is not null && help.Visibility == Visibility.Visible)
        {
            help.Measure(new Size(double.PositiveInfinity, double.PositiveInfinity));
            helpWidth = help.DesiredSize.Width + Spacing;
            helpHeight = help.DesiredSize.Height;
        }
        var mainAvailable = double.IsInfinity(availableSize.Width)
            ? double.PositiveInfinity
            : Math.Max(0, availableSize.Width - helpWidth);
        main?.Measure(new Size(mainAvailable, availableSize.Height));
        var mainSize = main?.DesiredSize ?? new Size(0, 0);
        return new Size(mainSize.Width + helpWidth, Math.Max(mainSize.Height, helpHeight));
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var (main, help) = Parts();
        var helpVisible = help is not null && help.Visibility == Visibility.Visible;
        var helpWidth = helpVisible ? help!.DesiredSize.Width + Spacing : 0;
        var room = Math.Max(0, finalSize.Width - helpWidth);
        double mainWidth = 0;
        if (main is not null)
        {
            mainWidth = main is FrameworkElement { HorizontalAlignment: HorizontalAlignment.Stretch }
                ? room
                : Math.Min(main.DesiredSize.Width, room);
            main.Arrange(new Rect(0, 0, mainWidth, finalSize.Height));
        }
        if (help is null) return finalSize;
        if (!helpVisible)
        {
            help.Arrange(new Rect(0, 0, 0, 0));
            return finalSize;
        }
        var size = help.DesiredSize;
        var alignment = (help as FrameworkElement)?.VerticalAlignment ?? VerticalAlignment.Bottom;
        var top = alignment switch
        {
            VerticalAlignment.Top => 0,
            VerticalAlignment.Center => Math.Max(0, (finalSize.Height - size.Height) / 2),
            _ => Math.Max(0, finalSize.Height - size.Height),
        };
        help.Arrange(new Rect(mainWidth + Spacing, top, size.Width, size.Height));
        return finalSize;
    }
}
