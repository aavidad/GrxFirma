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
        RefreshExportLabels();
        ViewModel.PropertyChanged += (_, change) =>
        {
            if (change.PropertyName is nameof(VerifyPageViewModel.HasHtmlReport)
                or nameof(VerifyPageViewModel.CanExportReport)) RefreshExportLabels();
            if (change.PropertyName == nameof(VerifyPageViewModel.ReportSavedMessage))
                AnnounceReportSaved();
        };
    }

    // El lector de pantalla no oye el cambio de texto por sí solo: se lanza
    // el evento cuando el enlace ya ha escrito el mensaje nuevo.
    private void AnnounceReportSaved()
    {
        if (string.IsNullOrEmpty(ViewModel.ReportSavedMessage)) return;
        _ = DispatcherQueue.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, () =>
        {
            if (string.IsNullOrEmpty(ReportSavedText.Text)) return;
            var peer = Microsoft.UI.Xaml.Automation.Peers.FrameworkElementAutomationPeer.FromElement(ReportSavedText) ??
                Microsoft.UI.Xaml.Automation.Peers.FrameworkElementAutomationPeer.CreatePeerForElement(ReportSavedText);
            peer?.RaiseAutomationEvent(Microsoft.UI.Xaml.Automation.Peers.AutomationEvents.LiveRegionChanged);
        });
    }

    public VerifyPageViewModel ViewModel { get; }

    // Con informe HTML del motor, ese es el botón principal y el JSON queda
    // como datos técnicos; sin él, el único botón exporta el JSON.
    private void RefreshExportLabels()
    {
        ExportReportButton.Content = Localizer.Text(ViewModel.HasHtmlReport || !ViewModel.CanExportReport
            ? "winui.parity.verify.export_html"
            : "winui.parity.verify.export");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            ExportReportButton, (string)ExportReportButton.Content);
        ExportJsonReportButton.Content = Localizer.Text("winui.parity.verify.export_json");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            ExportJsonReportButton, (string)ExportJsonReportButton.Content);
    }

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
        RoutedEventArgs args)
    {
        await ViewModel.VerifyAsync();
        // El foco va al resultado para que se lea; antes quedaba en un panel sin nombre.
        if (_isLoaded && ViewModel.HasResult)
        {
            ResultInfoBar.StartBringIntoView();
            ResultInfoBar.Focus(FocusState.Programmatic);
        }
    }

    private void OnCancelClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.CancelCurrentOperation();

    private async void OnExportReportClick(object sender, RoutedEventArgs args) =>
        await ViewModel.ExportReportAsync();

    private async void OnExportJsonReportClick(object sender, RoutedEventArgs args) =>
        await ViewModel.ExportJsonReportAsync();

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
