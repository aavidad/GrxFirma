// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Layout;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Foundation;

namespace GrxFirma.WinUI.Controls;

/// <summary>Conserva los botones nativos y los envuelve sin cambiar su orden de teclado.</summary>
public sealed class ResponsiveActionPanel : Panel
{
    public static readonly DependencyProperty OrientationProperty = DependencyProperty.Register(
        nameof(Orientation), typeof(Orientation), typeof(ResponsiveActionPanel),
        new PropertyMetadata(Orientation.Vertical, OnLayoutPropertyChanged));

    public static readonly DependencyProperty SpacingProperty = DependencyProperty.Register(
        nameof(Spacing), typeof(double), typeof(ResponsiveActionPanel),
        new PropertyMetadata(8d, OnLayoutPropertyChanged));

    public Orientation Orientation
    {
        get => (Orientation)GetValue(OrientationProperty);
        set => SetValue(OrientationProperty, value);
    }

    public double Spacing
    {
        get => (double)GetValue(SpacingProperty);
        set => SetValue(SpacingProperty, value);
    }

    private static void OnLayoutPropertyChanged(DependencyObject sender, DependencyPropertyChangedEventArgs args)
        => ((ResponsiveActionPanel)sender).InvalidateMeasure();

    protected override Size MeasureOverride(Size availableSize)
    {
        foreach (var child in Children)
        {
            child.Measure(new Size(availableSize.Width, double.PositiveInfinity));
        }
        var layout = Calculate(availableSize.Width);
        return new Size(layout.Width, layout.Height);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var layout = Calculate(finalSize.Width);
        var index = 0;
        foreach (var child in Children)
        {
            if (child.Visibility == Visibility.Collapsed)
            {
                child.Arrange(new Rect(0, 0, 0, 0));
                continue;
            }
            var item = layout.Items[index++];
            child.Arrange(new Rect(item.X, item.Y, item.Width, item.Height));
        }
        return finalSize;
    }

    private ActionFlowResult Calculate(double width) => ActionFlowLayout.Calculate(
        Children.Where(child => child.Visibility != Visibility.Collapsed)
            .Select(child => new ActionItemSize(child.DesiredSize.Width, child.DesiredSize.Height))
            .ToArray(),
        width,
        double.IsFinite(Spacing) ? Math.Max(0, Spacing) : 0,
        Orientation == Orientation.Horizontal);
}
