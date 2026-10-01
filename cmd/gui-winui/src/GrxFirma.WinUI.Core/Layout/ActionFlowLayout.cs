// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Layout;

public readonly record struct ActionItemSize(double Width, double Height);
public readonly record struct ActionItemBounds(double X, double Y, double Width, double Height);
public sealed record ActionFlowResult(
    double Width, double Height, IReadOnlyList<ActionItemBounds> Items);

/// <summary>Distribuye acciones en orden de lectura sin exceder el ancho disponible.</summary>
public static class ActionFlowLayout
{
    public static ActionFlowResult Calculate(
        IReadOnlyList<ActionItemSize> items,
        double availableWidth,
        double spacing,
        bool horizontal)
    {
        ArgumentNullException.ThrowIfNull(items);
        if (double.IsNaN(availableWidth) || availableWidth < 0 ||
            !double.IsFinite(spacing) || spacing < 0)
        {
            throw new ArgumentOutOfRangeException(nameof(availableWidth));
        }

        var bounds = new List<ActionItemBounds>(items.Count);
        double x = 0, y = 0, lineHeight = 0, usedWidth = 0;
        var hasItemInLine = false;
        foreach (var item in items)
        {
            if (!double.IsFinite(item.Width) || !double.IsFinite(item.Height) ||
                item.Width < 0 || item.Height < 0)
            {
                throw new ArgumentOutOfRangeException(nameof(items));
            }

            var width = Math.Min(item.Width, availableWidth);
            if (hasItemInLine && (!horizontal || x + spacing + width > availableWidth))
            {
                y += lineHeight + spacing;
                x = 0;
                lineHeight = 0;
                hasItemInLine = false;
            }
            if (hasItemInLine)
            {
                x += spacing;
            }

            var arrangedWidth = !horizontal && double.IsFinite(availableWidth)
                ? availableWidth
                : width;
            bounds.Add(new ActionItemBounds(x, y, arrangedWidth, item.Height));
            usedWidth = Math.Max(usedWidth, x + width);
            x += width;
            lineHeight = Math.Max(lineHeight, item.Height);
            hasItemInLine = true;
        }

        return new ActionFlowResult(usedWidth, y + lineHeight, bounds);
    }
}
