// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Layout;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class ActionFlowLayoutTests
{
    [TestMethod]
    public void ExactFitKeepsActionsOnSameRow()
    {
        var result = ActionFlowLayout.Calculate([new(100, 40), new(100, 48)], 208, 8, true);
        Assert.AreEqual(208d, result.Width);
        Assert.AreEqual(48d, result.Height);
        Assert.AreEqual(new ActionItemBounds(108, 0, 100, 48), result.Items[1]);
    }

    [TestMethod]
    public void OverflowWrapsUsingTallestItemAndKeepsOrder()
    {
        var result = ActionFlowLayout.Calculate([new(100, 40), new(100, 60), new(100, 42)], 220, 8, true);
        Assert.AreEqual(new ActionItemBounds(0, 68, 100, 42), result.Items[2]);
        Assert.AreEqual(110d, result.Height);
    }

    [TestMethod]
    public void SixCertificateActionsRemainWithinNarrowViewport()
    {
        var result = ActionFlowLayout.Calculate(Enumerable.Repeat(new ActionItemSize(210, 44), 6).ToArray(), 460, 8, true);
        Assert.AreEqual(148d, result.Height);
        Assert.IsTrue(result.Items.All(item => item.X + item.Width <= 460));
        Assert.AreEqual(new ActionItemBounds(218, 104, 210, 44), result.Items[5]);
    }

    [TestMethod]
    public void OversizedItemDoesNotAddAnEmptyFirstRow()
    {
        var result = ActionFlowLayout.Calculate([new(400, 60), new(100, 40)], 200, 8, true);
        Assert.AreEqual(new ActionItemBounds(0, 0, 200, 60), result.Items[0]);
        Assert.AreEqual(new ActionItemBounds(0, 68, 100, 40), result.Items[1]);
    }

    [TestMethod]
    public void VerticalLayoutStretchesWithoutHorizontalOverflow()
    {
        var result = ActionFlowLayout.Calculate([new(100, 40), new(150, 48)], 200, 8, false);
        Assert.AreEqual(new ActionItemBounds(0, 48, 200, 48), result.Items[1]);
        Assert.AreEqual(96d, result.Height);
    }

    [TestMethod]
    public void EmptyLayoutDoesNotReserveSpacing()
    {
        var result = ActionFlowLayout.Calculate([], 200, 8, true);
        Assert.AreEqual(0d, result.Width);
        Assert.AreEqual(0d, result.Height);
    }

    [TestMethod]
    public void ZeroWidthDoesNotCreateNegativeBounds()
    {
        var result = ActionFlowLayout.Calculate([new(100, 40), new(150, 48)], 0, 8, true);
        Assert.AreEqual(new ActionItemBounds(0, 48, 0, 48), result.Items[1]);
    }

    [TestMethod]
    public void UnboundedMeasurementRemainsFinite()
    {
        var result = ActionFlowLayout.Calculate([new(100, 40), new(150, 48)], double.PositiveInfinity, 8, true);
        Assert.AreEqual(258d, result.Width);
        Assert.AreEqual(48d, result.Height);
    }
}
