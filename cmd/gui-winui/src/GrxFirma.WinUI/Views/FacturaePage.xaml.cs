// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class FacturaePage : Page
{
    private bool _isLoaded;

    public FacturaePage()
    {
        var app = (App)Application.Current;
        ViewModel = new FacturaePageViewModel(
            new WindowsFacePortalLauncherService(),
            app.FilePickerService);
        InitializeComponent();
    }

    public FacturaePageViewModel ViewModel { get; }

    private void OnLoaded(
        object sender,
        RoutedEventArgs args)
    {
        if (_isLoaded)
        {
            return;
        }

        _isLoaded = true;
        ViewModel.Activate();
    }

    private void OnUnloaded(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded)
        {
            return;
        }

        _isLoaded = false;
        ViewModel.Deactivate();
    }

    private async void OnOpenValidatorClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenValidatorAsync();

    private async void OnCreateInvoiceClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.CreateInvoiceAsync();

    private async void OnOpenOrganisationDirectoryClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenOrganisationDirectoryAsync();

    private async void OnOpenSubmissionClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenSubmissionAsync();

    private async void OnOpenInvoiceStatusClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenInvoiceStatusAsync();

    private async void OnOpenReceiptVerificationClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenReceiptVerificationAsync();
}
