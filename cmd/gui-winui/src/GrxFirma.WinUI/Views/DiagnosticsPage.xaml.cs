// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class DiagnosticsPage : Page
{
    private bool _isLoaded;

    public DiagnosticsPage()
    {
        var app = (App)Application.Current;
        ViewModel = new DiagnosticsPageViewModel(
            app.OperationSession);
        InitializeComponent();
    }

    public DiagnosticsPageViewModel ViewModel { get; }

    private void OnLoaded(object sender, RoutedEventArgs args)
    {
        if (_isLoaded)
        {
            return;
        }

        _isLoaded = true;
        ViewModel.DiagnosticRequested += OnDiagnosticRequested;
        ViewModel.Activate();
    }

    private void OnUnloaded(object sender, RoutedEventArgs args)
    {
        if (!_isLoaded)
        {
            return;
        }

        _isLoaded = false;
        ViewModel.DiagnosticRequested -= OnDiagnosticRequested;
        ViewModel.Deactivate();
    }

    private async void OnRunDiagnosticClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.RunDiagnosticAsync();

    private void OnCancelClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.CancelCurrentOperation();

    private void OnOpenDetailedDiagnosticClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.RequestDetailedDiagnostic();

    private async void OnRefreshTlsClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.RefreshTlsStatusAsync();

    private async void OnInstallTlsTrustClick(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var confirmation = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = "Instalar confianza TLS local",
            Content =
                "GrxFirma creará, si es necesario, una CA local propia y la confiará únicamente para el usuario actual. No se modifican certificados personales ni raíces ajenas.",
            PrimaryButtonText = "Instalar confianza",
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        if (await confirmation.ShowAsync() ==
            ContentDialogResult.Primary)
        {
            await ViewModel.InstallTlsTrustAsync();
        }
    }

    private async void OnClearTlsTrustClick(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var confirmation = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = "Retirar confianza TLS local",
            Content =
                "Se retirará primero la CA local inventariada y después sus artefactos gestionados. La firma desde navegador dejará de funcionar hasta volver a instalarla.",
            PrimaryButtonText = "Retirar confianza",
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        if (await confirmation.ShowAsync() ==
            ContentDialogResult.Primary)
        {
            await ViewModel.ClearTlsTrustAsync();
        }
    }

    private async void OnDiagnosticRequested(
        OperationDiagnostic diagnostic)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var dialog = new OperationDiagnosticDialog(diagnostic)
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
        };
        await dialog.ShowAsync();
    }
}
