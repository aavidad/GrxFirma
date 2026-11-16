// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
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

    // Un apartado: título en seminegrita con nivel de encabezado 3 (el caso es
    // el 2) y su texto debajo, cada uno en su propio TextBlock.
    private static StackPanel HelpPanel(UnsafeExceptionState item)
    {
        var panel = new StackPanel { Spacing = 16 };
        var sections = UnsafeExceptionText.HelpSections(CatalogText(item.HelpKey),
            CatalogText("excepciones.pantalla.ayuda_organizacion"), UnsafeExceptionText.OrganizationDecides(item));
        foreach (var section in sections)
        {
            var block = new StackPanel { Spacing = 4 };
            if (section.Title.Length > 0)
            {
                var title = new TextBlock
                {
                    Text = section.Title,
                    TextWrapping = TextWrapping.Wrap,
                    FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
                };
                AutomationProperties.SetHeadingLevel(title, AutomationHeadingLevel.Level3);
                block.Children.Add(title);
            }
            block.Children.Add(new TextBlock { Text = section.Body, TextWrapping = TextWrapping.Wrap });
            panel.Children.Add(block);
        }
        return panel;
    }

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
        var risk = Prose(item.RiskKey);
        body.Children.Add(risk);
        // La ayuda larga se lee por apartados, cada uno con su título como
        // encabezado: el lector de pantalla salta de uno a otro con la tecla H
        // del modo de examen. Si lo decide la organización, no explica cómo
        // activar ni deshacer: dice quién lo decide (lote 39h).
        var help = HelpPanel(item);
        body.Children.Add(help);
        var organization = Prose(item.OrganizationKey);
        body.Children.Add(organization);
        // Con la ayuda abierta, el texto de la organización ya va en su
        // apartado y no se repite debajo.
        void ShowHelp(bool visible)
        {
            help.Visibility = visible && help.Children.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
            risk.Visibility = visible && help.Children.Count > 0 ? Visibility.Collapsed : Visibility.Visible;
            if (visible && help.Children.Count == 0) risk.Text = CatalogText(item.HelpKey);
            organization.Visibility = !visible && !item.Activable && organization.Text.Length > 0
                ? Visibility.Visible : Visibility.Collapsed;
        }
        ShowHelp(helpOnly);
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
        // El texto del caso es la primera parada del teclado y el foco inicial:
        // el lector de pantalla empieza por el nombre del caso, como en
        // Android, y las flechas desplazan la ayuda larga. Antes el foco caía
        // en «Más información».
        var scroll = new ScrollViewer
        {
            Content = body,
            VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
            HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
            HorizontalScrollMode = ScrollMode.Disabled,
            MaxHeight = Math.Max(120, root.Size.Height * 0.6),
            IsTabStop = true,
            UseSystemFocusVisuals = true,
            TabIndex = 0,
        };
        AutomationProperties.SetName(scroll, heading.Text);
        // Con la ayuda abierta el texto de ayuda queda vacío: antes era la
        // ayuda entera, que el lector leía como un solo bloque.
        AutomationProperties.SetHelpText(scroll, helpOnly ? string.Empty : risk.Text);
        var dialog = new ContentDialog
        {
            XamlRoot = root,
            RequestedTheme = theme,
            Title = Localizer.Text(model.TitleKey(helpOnly)),
            Content = scroll,
            CloseButtonText = Localizer.Text(model.CloseButtonKey(helpOnly)),
            PrimaryButtonText = !helpOnly && item.Activable ? Localizer.Text(model.PrimaryButtonKey) : string.Empty,
            IsPrimaryButtonEnabled = false,
            // El botón principal se distingue por su estilo, no por ser el
            // predeterminado: Intro sobre la casilla no debe activar nada.
            PrimaryButtonStyle = Application.Current.Resources.TryGetValue("AccentButtonStyle", out var accent)
                ? accent as Style : null,
            DefaultButton = ContentDialogButton.None,
        };
        AutomationProperties.SetName(dialog, dialog.Title as string ?? string.Empty);
        AutomationProperties.SetHelpText(dialog, CatalogText(item.RiskKey));
        acceptance.Checked += (_, _) => { model.Accepted = true; dialog.IsPrimaryButtonEnabled = model.CanActivate; };
        acceptance.Unchecked += (_, _) => { model.Accepted = false; dialog.IsPrimaryButtonEnabled = false; };
        if (!helpOnly)
        {
            var more = new Button { Content = Localizer.Text("excepciones.mas_informacion"), MinHeight = 44 };
            more.Click += (_, _) =>
            {
                ShowHelp(true);
                AutomationProperties.SetHelpText(scroll, string.Empty);
                // El botón desaparece: el foco pasa al texto, que ya es la ayuda.
                scroll.Focus(FocusState.Keyboard);
                more.Visibility = Visibility.Collapsed;
                help.StartBringIntoView();
            };
            body.Children.Insert(3, more);
        }
        dialog.Opened += (_, _) =>
        {
            // La cola común de diálogos puede abrirlo cuando ya se ha cancelado.
            if (cancellationToken.IsCancellationRequested) { dialog.Hide(); return; }
            // ContentDialog pone su propio foco al abrirse; el nuestro va
            // después, en la cola, para que no lo pise.
            dialog.DispatcherQueue.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low,
                () => scroll.Focus(FocusState.Keyboard));
        };
        cancellationToken.ThrowIfCancellationRequested();
        using var cancellation = cancellationToken.Register(() => dialog.DispatcherQueue.TryEnqueue(() => dialog.Hide()));
        // Localizer.ShowAsync comparte la cola de diálogos de toda la app:
        // WinUI no admite dos ContentDialog abiertos a la vez.
        return await Localizer.ShowAsync(dialog) == ContentDialogResult.Primary && model.CanActivate && !cancellationToken.IsCancellationRequested;
    }
}
