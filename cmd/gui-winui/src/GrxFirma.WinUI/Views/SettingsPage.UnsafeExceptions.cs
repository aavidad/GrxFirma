// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class SettingsPage
{
    private async Task LoadUnsafeExceptionsAsync()
    {
        UnsafeHeading.Text = Localizer.Text("excepciones.pantalla.preferencias");
        UnsafeDescription.Text = Localizer.Text("excepciones.pantalla.descripcion");
        UnsafeRefresh.Content = Localizer.Text("excepciones.pantalla.actualizar");
        UnsafeRefresh.IsEnabled = false;
        var client = ((App)Application.Current).UnsafeExceptions;
        UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.cargando");
        try
        {
            if (client is null) { UnsafeMessage.Text = Localizer.Text("excepciones.canal_no_local"); return; }
            var result = await client.ListAsync(_pageCancellation?.Token ?? default);
            if (!_isLoaded) return;
            if (!result.IsSuccess || result.Data is null)
            { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); return; }
            UnsafeRows.Children.Clear();
            foreach (var item in result.Data)
            {
                var row = new StackPanel { Spacing = 8 };
                TextBlock Prose(string text) => new() { Text = text, TextWrapping = TextWrapping.Wrap };
                var setting = UnsafeExceptionDialog.CatalogText(item.SettingKey);
                var title = Prose(setting);
                title.FontWeight = Microsoft.UI.Text.FontWeights.SemiBold;
                AutomationProperties.SetHeadingLevel(title, AutomationHeadingLevel.Level3);
                row.Children.Add(title);
                var status = item.StatusKey == "excepciones.pantalla.activada_fecha"
                    ? Localizer.Fill(item.StatusKey, ("date", item.LastAcceptance!.AcceptedAt.ToLocalTime().ToString("g", CultureInfo.CurrentCulture)))
                    : Localizer.Text(item.StatusKey);
                row.Children.Add(Prose(status));
                var organization = UnsafeExceptionDialog.CatalogText(item.OrganizationKey);
                if (item.Fixed || !item.Activable)
                    row.Children.Add(Prose(item.PolicyUnreadable ? Localizer.Text("excepciones.pantalla.directiva_ilegible")
                        : Localizer.Text("excepciones.pantalla.solo_lectura") +
                        (item.Enabled || organization.Length == 0 ? string.Empty : Environment.NewLine + organization)));
                var actions = new ResponsiveActionPanel { Spacing = 12 };
                var change = new Button
                {
                    Content = Localizer.Text(item.Enabled ? "excepciones.pantalla.desactivar" : "excepciones.pantalla.activar"),
                    Visibility = item.Enabled || (item.Activable && !item.Fixed) ? Visibility.Visible : Visibility.Collapsed,
                    MinHeight = 44, IsEnabled = !item.Fixed && (item.Enabled || item.Activable),
                    // Mismo índice que «Actualizar lista»: a igualdad, el foco
                    // sigue el orden visual y no salta al final de la página.
                    TabIndex = UnsafeRefresh.TabIndex,
                };
                AutomationProperties.SetHelpText(change, setting);
                change.Click += async (_, _) =>
                {
                    change.IsEnabled = false;
                    try
                    {
                        if (item.Enabled)
                        {
                            var disabled = await client.DisableAsync(item, _pageCancellation?.Token ?? default);
                            if (!disabled.IsSuccess) { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); return; }
                        }
                        else if (!await client.ActivateAsync(item, ct: _pageCancellation?.Token ?? default)) return;
                        await LoadUnsafeExceptionsAsync();
                    }
                    catch (OperationCanceledException) { }
                    catch { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); }
                    finally { change.IsEnabled = !item.Fixed && (item.Enabled || item.Activable); }
                };
                var help = new Button
                {
                    Content = Localizer.Text("excepciones.mas_informacion"), MinHeight = 44,
                    TabIndex = UnsafeRefresh.TabIndex,
                };
                AutomationProperties.SetHelpText(help, setting);
                help.Click += async (_, _) => await UnsafeExceptionDialog.ShowAsync(item, XamlRoot, ActualTheme, true);
                actions.Children.Add(change);
                actions.Children.Add(help);
                row.Children.Add(actions);
                UnsafeRows.Children.Add(row);
            }
            UnsafeMessage.Text = string.Empty;
        }
        catch (OperationCanceledException) { }
        catch { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); }
        finally { UnsafeRefresh.IsEnabled = client is not null; }
    }
    private async void OnUnsafeRefresh(object sender, RoutedEventArgs args) => await LoadUnsafeExceptionsAsync();
}
