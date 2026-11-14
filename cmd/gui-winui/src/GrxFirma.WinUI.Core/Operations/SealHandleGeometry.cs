// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public sealed record SealHandleTarget(double X, double Y, double Size);
public sealed record SealHandleTargets(SealHandleTarget Rotate, SealHandleTarget Resize);

public static class SealHandleGeometry
{
    // Todas las coordenadas están en el espacio del PDF; Scale lleva a DIP.
    public static SealHandleTargets Calculate(double scale, double width, double height,
        double rotateX, double rotateY, double resizeX, double resizeY)
    {
        if (!double.IsFinite(scale) || scale <= 0)
            throw new ArgumentOutOfRangeException(nameof(scale));
        var rotateSize = Math.Max(44, 24 / scale);
        var resizeSize = Math.Max(36, 24 / scale);
        SealHandleTarget Clamp(double x, double y, double size) => new(
            Math.Clamp(x, 0, Math.Max(0, width - size)),
            Math.Clamp(y, 0, Math.Max(0, height - size)), size);
        var rotate = Clamp(rotateX - rotateSize / 2, rotateY - rotateSize / 2, rotateSize);
        var resize = Clamp(resizeX - resizeSize / 2, resizeY - resizeSize / 2, resizeSize);
        static bool Overlap(SealHandleTarget a, SealHandleTarget b) =>
            a.X < b.X + b.Size && a.X + a.Size > b.X &&
            a.Y < b.Y + b.Size && a.Y + a.Size > b.Y;
        if (Overlap(rotate, resize))
        {
            var gap = 2 / scale;
            // Mantener ambos objetivos utilizables incluso con sellos mínimos
            // y cuando las esquinas giradas caen junto al borde de la página.
            SealHandleTarget[] candidates =
            [
                Clamp(resize.X, rotate.Y + rotate.Size + gap, resizeSize),
                Clamp(resize.X, rotate.Y - resizeSize - gap, resizeSize),
                Clamp(rotate.X - resizeSize - gap, resize.Y, resizeSize),
                Clamp(rotate.X + rotate.Size + gap, resize.Y, resizeSize),
            ];
            resize = candidates.FirstOrDefault(candidate => !Overlap(rotate, candidate)) ?? resize;
        }
        return new(rotate, resize);
    }
}
