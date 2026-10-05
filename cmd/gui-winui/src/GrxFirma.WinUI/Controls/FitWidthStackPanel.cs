// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using Windows.Foundation;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

// Pila vertical que no deja a ningún hijo pedir más ancho del disponible.
// Un desplegable pide el ancho de su elemento más largo y, en una pila
// normal, ese ancho subía hasta la ventana y cortaba la página por la
// derecha (ENI a 1280 px, WCAG 1.4.10). Aquí cada hijo se mide y se coloca
// como mucho con el ancho que recibe la pila: sin mínimo fijo, se ajusta al
// contenedor.
public sealed class FitWidthStackPanel : Panel
{
    public double Spacing { get; set; }

    protected override Size MeasureOverride(Size availableSize)
    {
        var limited = !double.IsInfinity(availableSize.Width) && availableSize.Width > 0;
        double width = 0, height = 0;
        var visible = 0;
        foreach (var child in Children)
        {
            if (limited && child is FrameworkElement element)
            {
                var limit = Math.Max(0, availableSize.Width - element.Margin.Left - element.Margin.Right);
                if (element.MinWidth > limit) element.MinWidth = 0;
                if (element.MaxWidth != limit) element.MaxWidth = limit;
            }
            child.Measure(new Size(availableSize.Width, double.PositiveInfinity));
            if (child.Visibility == Visibility.Collapsed) continue;
            var desired = child.DesiredSize;
            width = Math.Max(width, limited ? Math.Min(desired.Width, availableSize.Width) : desired.Width);
            height += desired.Height + (visible++ > 0 ? Spacing : 0);
        }
        return new Size(width, height);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        double top = 0;
        var visible = 0;
        foreach (var child in Children)
        {
            if (child.Visibility == Visibility.Collapsed)
            {
                child.Arrange(new Rect(0, top, 0, 0));
                continue;
            }
            if (visible++ > 0) top += Spacing;
            var height = child.DesiredSize.Height;
            child.Arrange(new Rect(0, top, finalSize.Width, height));
            top += height;
        }
        return new Size(finalSize.Width, Math.Max(top, 0));
    }
}
