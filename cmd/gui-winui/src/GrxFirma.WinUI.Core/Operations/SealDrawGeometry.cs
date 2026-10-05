// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public readonly record struct SealDrawRect(double X, double Y, double Width, double Height);

public static class SealDrawGeometry
{
    // Puntos normalizados desde arriba en la vista; Y desde abajo en el PDF.
    public const double MinimumSize = 0.02;

    public static SealDrawRect Normalize(double x1, double y1, double x2, double y2)
    {
        if (!double.IsFinite(x1)) throw new ArgumentOutOfRangeException(nameof(x1));
        if (!double.IsFinite(y1)) throw new ArgumentOutOfRangeException(nameof(y1));
        if (!double.IsFinite(x2)) throw new ArgumentOutOfRangeException(nameof(x2));
        if (!double.IsFinite(y2)) throw new ArgumentOutOfRangeException(nameof(y2));
        x1 = Math.Clamp(x1, 0, 1); y1 = Math.Clamp(y1, 0, 1);
        x2 = Math.Clamp(x2, 0, 1); y2 = Math.Clamp(y2, 0, 1);
        return new(Math.Min(x1, x2), 1 - Math.Max(y1, y2),
            Math.Abs(x2 - x1), Math.Abs(y2 - y1));
    }

    public static bool IsLargeEnough(SealDrawRect rect) =>
        rect.Width + 1e-12 >= MinimumSize && rect.Height + 1e-12 >= MinimumSize;
}
