// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class SealHandleGeometryTests
{
    [TestMethod]
    public void TargetsKeep24DipStayInsidePageAndDoNotOverlap()
    {
        foreach (var scale in new[] { 0.1, 0.25, 0.5, 1.0, 1.5, 2.0, 4.0 })
        foreach (var corner in new[] { (0.0, 0.0), (612.0, 0.0), (612.0, 792.0), (0.0, 792.0), (300.0, 400.0) })
        {
            // Esquinas coincidentes: el sello mínimo es el peor caso.
            var result = SealHandleGeometry.Calculate(scale, 612, 792,
                corner.Item1, corner.Item2, corner.Item1, corner.Item2);
            foreach (var target in new[] { result.Rotate, result.Resize })
            {
                Assert.IsTrue(target.Size * scale >= 24 - 0.0001);
                Assert.IsTrue(target.X >= 0 && target.Y >= 0);
                Assert.IsTrue(target.X + target.Size <= 612);
                Assert.IsTrue(target.Y + target.Size <= 792);
            }
            var a = result.Rotate;
            var b = result.Resize;
            Assert.IsFalse(a.X < b.X + b.Size && a.X + a.Size > b.X &&
                a.Y < b.Y + b.Size && a.Y + a.Size > b.Y);
        }
    }

    [TestMethod]
    public void InvalidScaleIsRejected()
    {
        foreach (var scale in new[] { 0.0, -1.0, double.NaN, double.PositiveInfinity })
            Assert.ThrowsExactly<ArgumentOutOfRangeException>(() =>
                SealHandleGeometry.Calculate(scale, 612, 792, 0, 0, 0, 0));
    }
}
