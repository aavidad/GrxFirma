// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class SealDrawGeometryTests
{
    [TestMethod]
    public void Normalize_UsesBottomOriginInAllFourDirections()
    {
        foreach (var (x1, y1, x2, y2) in new[] {
            (0.2, 0.3, 0.7, 0.8), (0.7, 0.3, 0.2, 0.8),
            (0.2, 0.8, 0.7, 0.3), (0.7, 0.8, 0.2, 0.3) })
        {
            var rect = SealDrawGeometry.Normalize(x1, y1, x2, y2);
            Assert.AreEqual(0.2, rect.X, 1e-12);
            Assert.AreEqual(0.2, rect.Y, 1e-12);
            Assert.AreEqual(0.5, rect.Width, 1e-12);
            Assert.AreEqual(0.5, rect.Height, 1e-12);
        }
    }

    [TestMethod]
    public void Normalize_ClampsBothCornersBeforeComputingSize()
    {
        Assert.AreEqual(new SealDrawRect(0, 0, 1, 1), SealDrawGeometry.Normalize(-2, 5, 4, -3));
        Assert.AreEqual(new SealDrawRect(1, 1, 0, 0), SealDrawGeometry.Normalize(2, -2, 3, -1));
        var rect = SealDrawGeometry.Normalize(0.95, 0.95, 2, 2);
        Assert.AreEqual(0.95, rect.X, 1e-12);
        Assert.AreEqual(0, rect.Y, 1e-12);
        Assert.AreEqual(0.05, rect.Width, 1e-12);
        Assert.AreEqual(0.05, rect.Height, 1e-12);
    }

    [TestMethod]
    public void Minimum_MatchesTheTwoPercentResizeLimit()
    {
        Assert.IsTrue(SealDrawGeometry.IsLargeEnough(SealDrawGeometry.Normalize(0.6, 0.6, 0.62, 0.62)));
        foreach (var (x2, y2) in new[] { (0.61, 0.8), (0.8, 0.61), (0.6, 0.6) })
            Assert.IsFalse(SealDrawGeometry.IsLargeEnough(SealDrawGeometry.Normalize(0.6, 0.6, x2, y2)));
    }

    [TestMethod]
    public void Normalize_RejectsNonFiniteInputExactly()
    {
        foreach (var invalid in new[] { double.NaN, double.PositiveInfinity, double.NegativeInfinity })
        {
            Assert.ThrowsExactly<ArgumentOutOfRangeException>(() => SealDrawGeometry.Normalize(invalid, 0, 1, 1));
            Assert.ThrowsExactly<ArgumentOutOfRangeException>(() => SealDrawGeometry.Normalize(0, invalid, 1, 1));
            Assert.ThrowsExactly<ArgumentOutOfRangeException>(() => SealDrawGeometry.Normalize(0, 0, invalid, 1));
            Assert.ThrowsExactly<ArgumentOutOfRangeException>(() => SealDrawGeometry.Normalize(0, 0, 1, invalid));
        }
    }
}
