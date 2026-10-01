// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Controls;

public static class CertificateCardSelection
{
    public static void Update(
        ListView list,
        SelectionChangedEventArgs args,
        Brush accentBrush)
    {
        foreach (var item in args.RemovedItems)
        {
            SetSelection(list.ContainerFromItem(item) as ListViewItem, false, accentBrush);
        }
        foreach (var item in args.AddedItems)
        {
            SetSelection(list.ContainerFromItem(item) as ListViewItem, true, accentBrush);
        }
    }

    public static void UpdateContainer(
        ListView list,
        ContainerContentChangingEventArgs args,
        Brush accentBrush)
    {
        if (!args.InRecycleQueue)
        {
            SetSelection(
                args.ItemContainer as ListViewItem,
                Equals(args.Item, list.SelectedItem),
                accentBrush);
        }
    }

    private static void SetSelection(
        ListViewItem? container,
        bool isSelected,
        Brush accentBrush)
    {
        if (container is null)
        {
            return;
        }
        if (isSelected)
        {
            container.BorderBrush = accentBrush;
            container.BorderThickness = new Thickness(2);
        }
        else
        {
            container.ClearValue(Control.BorderBrushProperty);
            container.ClearValue(Control.BorderThicknessProperty);
        }
    }
}
