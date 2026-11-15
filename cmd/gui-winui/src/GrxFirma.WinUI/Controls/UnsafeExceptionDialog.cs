// SPDX-License-Identifier: EUPL-1.2
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

internal static class UnsafeExceptionDialog
{
    // ContentDialog supplies focus containment, Escape, UIA dialog semantics and
    // focus restoration. The whole body scrolls; action buttons stay reachable.
    private static readonly SemaphoreSlim DisplayGate = new(1, 1);
    public static async Task<bool> ShowAsync(UnsafeExceptionState item, XamlRoot root,
        ElementTheme theme, bool helpOnly = false, CancellationToken cancellationToken = default)
    {
        await DisplayGate.WaitAsync(cancellationToken);
        try { return await ShowCoreAsync(item, root, theme, helpOnly, cancellationToken); }
        finally { DisplayGate.Release(); }
    }
    private static async Task<bool> ShowCoreAsync(UnsafeExceptionState item, XamlRoot root,
        ElementTheme theme, bool helpOnly, CancellationToken cancellationToken)
    {
        var model = new UnsafeExceptionConsentViewModel(item);
        var body = new StackPanel { Spacing = 16, MaxWidth = 680 };
        TextBlock Prose(string key) => new() { Text = Localizer.Text(key), TextWrapping = TextWrapping.Wrap };
        var heading = Prose(item.Key("ajuste"));
        heading.FontWeight = Microsoft.UI.Text.FontWeights.SemiBold;
        AutomationProperties.SetHeadingLevel(heading, AutomationHeadingLevel.Level2);
        body.Children.Add(heading);
        var risk = Prose(item.Key(helpOnly ? "ayuda" : "riesgo"));
        if (helpOnly) risk.Text = risk.Text.Replace("\n", "\n\n", StringComparison.Ordinal);
        body.Children.Add(risk);
        var organization = Prose(item.Key("organizacion"));
        organization.Visibility = !helpOnly && !item.Activable ? Visibility.Visible : Visibility.Collapsed;
        body.Children.Add(organization);
        var acceptance = new CheckBox
        {
            Content = Prose(item.Key("aceptacion")),
            HorizontalContentAlignment = HorizontalAlignment.Stretch,
            MinHeight = 44,
            Visibility = !helpOnly && item.Activable ? Visibility.Visible : Visibility.Collapsed,
        };
        AutomationProperties.SetName(acceptance, Localizer.Text(item.Key("aceptacion")));
        AutomationProperties.SetHelpText(acceptance, Localizer.Fill("excepciones.pantalla.version", ("version", item.AcceptanceVersion)));
        body.Children.Add(acceptance);
        if (item.Activable) body.Children.Add(Prose("excepciones.pantalla.desactivar_aqui"));
        var dialog = new ContentDialog
        {
            XamlRoot = root,
            RequestedTheme = theme,
            Title = Localizer.Text(helpOnly ? "excepciones.mas_informacion" : item.OperationPending ? "excepciones.pantalla.titulo" : "excepciones.pantalla.aceptar_titulo"),
            Content = new ScrollViewer
            {
                Content = body,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
                HorizontalScrollMode = ScrollMode.Disabled,
                MaxHeight = Math.Max(120, root.Size.Height * 0.6),
            },
            CloseButtonText = Localizer.Text(helpOnly ? "excepciones.pantalla.cerrar" : "excepciones.cancelar"),
            PrimaryButtonText = !helpOnly && item.Activable ? Localizer.Text("excepciones.activar_continuar") : string.Empty,
            IsPrimaryButtonEnabled = false,
            DefaultButton = ContentDialogButton.None,
        };
        AutomationProperties.SetName(dialog, dialog.Title as string ?? string.Empty);
        AutomationProperties.SetHelpText(dialog, Localizer.Text(item.Key("riesgo")));
        acceptance.Checked += (_, _) => { model.Accepted = true; dialog.IsPrimaryButtonEnabled = model.CanActivate; };
        acceptance.Unchecked += (_, _) => { model.Accepted = false; dialog.IsPrimaryButtonEnabled = false; };
        if (!helpOnly)
        {
            var help = new Button { Content = Localizer.Text("excepciones.mas_informacion"), MinHeight = 44 };
            help.Click += (_, _) =>
            {
                risk.Text = Localizer.Text(item.Key("ayuda")).Replace("\n", "\n\n", StringComparison.Ordinal);
                help.Visibility = Visibility.Collapsed;
                risk.StartBringIntoView();
            };
            body.Children.Insert(2, help);
        }
        dialog.Opened += (_, _) =>
        {
            if (acceptance.Visibility == Visibility.Visible) acceptance.Focus(FocusState.Programmatic);
            else dialog.Focus(FocusState.Programmatic);
        };
        cancellationToken.ThrowIfCancellationRequested();
        using var cancellation = cancellationToken.Register(() => dialog.DispatcherQueue.TryEnqueue(() => dialog.Hide()));
        return await dialog.ShowAsync() == ContentDialogResult.Primary && model.CanActivate && !cancellationToken.IsCancellationRequested;
    }
}
