// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Controls;

public static class CertificateValidationDialog
{
    public static async Task ShowAsync(
        CertificateListItem certificate,
        XamlRoot xamlRoot,
        ElementTheme theme)
    {
        ArgumentNullException.ThrowIfNull(certificate);

        var content = new StackPanel { Spacing = 12 };
        content.Children.Add(new TextBlock
        {
            Text = certificate.DisplayName,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            TextWrapping = TextWrapping.Wrap,
        });
        content.Children.Add(new TextBlock
        {
            Text = certificate.SuitabilitySummary,
            TextWrapping = TextWrapping.Wrap,
            Foreground = (Brush)new CertificateStatusBrushConverter().Convert(
                certificate.CardStatus,
                typeof(Brush),
                string.Empty,
                string.Empty),
        });
        content.Children.Add(new TextBlock
        {
            Text = Localizer.Text("winui.certificados.la_aptitud_se_basa_en_la_informacion"),
            TextWrapping = TextWrapping.Wrap,
        });
        content.Children.Add(new TextBlock
        {
            Text = Localizer.Text("winui.certificados.ficha_completa_del_certificado"),
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            TextWrapping = TextWrapping.Wrap,
        });
        foreach (var row in certificate.Details)
        {
            var pair = new StackPanel { Spacing = 2 };
            pair.Children.Add(new TextBlock
            {
                Text = row.Label,
                TextWrapping = TextWrapping.Wrap,
                Foreground = (Brush)Application.Current.Resources[
                    "AppMutedTextBrush"],
            });
            pair.Children.Add(new TextBlock
            {
                Text = row.Value,
                TextWrapping = TextWrapping.Wrap,
                IsTextSelectionEnabled = true,
            });
            content.Children.Add(pair);
        }

        var dialog = new ContentDialog
        {
            XamlRoot = xamlRoot,
            RequestedTheme = theme,
            Title = Localizer.Text("winui.certificados.verificar_certificado"),
            Content = new ScrollViewer
            {
                MaxHeight = 520,
                Content = content,
            },
            CloseButtonText = Localizer.Text("winui.comun.cerrar"),
            DefaultButton = ContentDialogButton.Close,
        };
        AutomationProperties.SetName(dialog, Localizer.Text("winui.certificados.verificacion_del_certificado"));
        await Localizer.ShowAsync(dialog);
    }
}
