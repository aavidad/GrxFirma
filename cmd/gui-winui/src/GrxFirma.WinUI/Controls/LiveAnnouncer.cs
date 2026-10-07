// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.CompilerServices;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

// Un TextBlock con AutomationProperties.LiveSetting no avisa por sí solo al
// lector de pantalla: hay que lanzar LiveRegionChanged cuando cambia. En XAML
// basta con controls:LiveAnnouncer.Watch="True"; en código, Watch() o
// Announce(). El aviso se aplaza a baja prioridad para que el enlace ya haya
// escrito el texto nuevo, y se agrupa si cambian texto y visibilidad a la vez.
public static class LiveAnnouncer
{
    public static readonly DependencyProperty WatchProperty =
        DependencyProperty.RegisterAttached(
            nameof(Watch), typeof(bool), typeof(LiveAnnouncer),
            new PropertyMetadata(false, OnWatchChanged));

    // Elementos con un aviso ya en cola.
    private static readonly ConditionalWeakTable<FrameworkElement, object> s_pending = new();

    public static bool GetWatch(DependencyObject element) =>
        (bool)element.GetValue(WatchProperty);

    public static void SetWatch(DependencyObject element, bool value) =>
        element.SetValue(WatchProperty, value);

    public static void Watch(params TextBlock[] elements)
    {
        foreach (var element in elements) SetWatch(element, true);
    }

    private static void OnWatchChanged(DependencyObject sender, DependencyPropertyChangedEventArgs args)
    {
        if (sender is not TextBlock text || args.NewValue is not true) return;
        text.RegisterPropertyChangedCallback(TextBlock.TextProperty, OnObservedChanged);
        text.RegisterPropertyChangedCallback(UIElement.VisibilityProperty, OnObservedChanged);
    }

    // Los valores iniciales llegan antes de Loaded y no se anuncian.
    private static void OnObservedChanged(DependencyObject sender, DependencyProperty property)
    {
        if (sender is FrameworkElement element && element.IsLoaded) Announce(element);
    }

    public static void Announce(FrameworkElement element)
    {
        if (!AutomationPeer.ListenerExists(AutomationEvents.LiveRegionChanged)) return;
        var queue = element.DispatcherQueue;
        if (queue is null || !s_pending.TryAdd(element, element)) return;
        if (!queue.TryEnqueue(DispatcherQueuePriority.Low, () => Raise(element)))
            s_pending.Remove(element);
    }

    private static void Raise(FrameworkElement element)
    {
        s_pending.Remove(element);
        if (element.XamlRoot is null || element.Visibility != Visibility.Visible) return;
        if (element is TextBlock text && string.IsNullOrWhiteSpace(text.Text)) return;
        var peer = FrameworkElementAutomationPeer.FromElement(element) ??
            FrameworkElementAutomationPeer.CreatePeerForElement(element);
        peer?.RaiseAutomationEvent(AutomationEvents.LiveRegionChanged);
    }
}
