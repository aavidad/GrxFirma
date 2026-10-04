// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using GrxFirma.WinUI.Services;

namespace GrxFirma.WinUI.Views;

public sealed partial class VerifyPage : Page
{
    private bool _isLoaded;

    public VerifyPage()
    {
        var app = (App)Application.Current;
        ViewModel = new VerifyPageViewModel(
            app.OperationSession,
            app.FilePickerService);
        InitializeComponent();
        ExportReportButton.Content = Localizer.Text("winui.parity.verify.export");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            ExportReportButton, (string)ExportReportButton.Content);
    }

    public VerifyPageViewModel ViewModel { get; }

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

    private async void OnSelectSignedFileClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.SelectSignedFileAsync();

    private async void OnSelectOriginalFileClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.SelectOriginalFileAsync();

    private void OnClearOriginalFileClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.ClearOriginalFile();

    private async void OnVerifyClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.VerifyAsync();

    private void OnCancelClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.CancelCurrentOperation();

    private async void OnExportReportClick(object sender, RoutedEventArgs args) =>
        await ViewModel.ExportReportAsync();

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
        await Localizer.ShowAsync(dialog);
    }
}
