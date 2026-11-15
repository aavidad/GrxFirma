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

    // Solo se presentan claves del catálogo. Si el motor no envía una clave o
    // el catálogo no la tiene, no se muestra nada en lugar de la clave cruda.
    internal static string CatalogText(string key) =>
        !string.IsNullOrWhiteSpace(key) && Localizer.Has(key) ? Localizer.Text(key) : string.Empty;

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
        TextBlock Prose(string key) => new() { Text = CatalogText(key), TextWrapping = TextWrapping.Wrap };
        var acceptanceText = CatalogText(item.AcceptanceKey);
        var heading = Prose(item.SettingKey);
        heading.FontWeight = Microsoft.UI.Text.FontWeights.SemiBold;
        AutomationProperties.SetHeadingLevel(heading, AutomationHeadingLevel.Level2);
        body.Children.Add(heading);
        var risk = Prose(helpOnly ? item.HelpKey : item.RiskKey);
        body.Children.Add(risk);
        var organization = Prose(item.OrganizationKey);
        organization.Visibility = !helpOnly && !item.Activable && organization.Text.Length > 0
            ? Visibility.Visible : Visibility.Collapsed;
        body.Children.Add(organization);
        var acceptance = new CheckBox
        {
            Content = Prose(item.AcceptanceKey),
            HorizontalContentAlignment = HorizontalAlignment.Stretch,
            MinHeight = 44,
            // Sin el texto de aceptación no hay nada que aceptar: la casilla no
            // aparece y el botón de activar sigue desactivado.
            Visibility = !helpOnly && item.Activable && acceptanceText.Length > 0 ? Visibility.Visible : Visibility.Collapsed,
        };
        AutomationProperties.SetName(acceptance, acceptanceText);
        AutomationProperties.SetHelpText(acceptance, Localizer.Fill("excepciones.pantalla.version", ("version", item.AcceptanceVersion)));
        body.Children.Add(acceptance);
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
        AutomationProperties.SetHelpText(dialog, CatalogText(item.RiskKey));
        acceptance.Checked += (_, _) => { model.Accepted = true; dialog.IsPrimaryButtonEnabled = model.CanActivate; };
        acceptance.Unchecked += (_, _) => { model.Accepted = false; dialog.IsPrimaryButtonEnabled = false; };
        if (!helpOnly)
        {
            var help = new Button { Content = Localizer.Text("excepciones.mas_informacion"), MinHeight = 44 };
            help.Click += (_, _) =>
            {
                risk.Text = CatalogText(item.HelpKey);
                help.Visibility = Visibility.Collapsed;
                risk.StartBringIntoView();
            };
            body.Children.Insert(2, help);
        }
        dialog.Opened += (_, _) =>
        {
            // La cola común de diálogos puede abrirlo cuando ya se ha cancelado.
            if (cancellationToken.IsCancellationRequested) { dialog.Hide(); return; }
            if (acceptance.Visibility == Visibility.Visible) acceptance.Focus(FocusState.Programmatic);
            else dialog.Focus(FocusState.Programmatic);
        };
        cancellationToken.ThrowIfCancellationRequested();
        using var cancellation = cancellationToken.Register(() => dialog.DispatcherQueue.TryEnqueue(() => dialog.Hide()));
        // Localizer.ShowAsync comparte la cola de diálogos de toda la app:
        // WinUI no admite dos ContentDialog abiertos a la vez.
        return await Localizer.ShowAsync(dialog) == ContentDialogResult.Primary && model.CanActivate && !cancellationToken.IsCancellationRequested;
    }
}
