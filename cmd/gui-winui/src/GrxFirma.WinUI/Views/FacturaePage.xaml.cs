// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.Core.Operations;
using System.Globalization;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class FacturaePage : Page
{
    private bool _isLoaded;
    private string _invoiceReport = string.Empty;
    private static string T(string key) => SealUiCatalog.Text(CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, key);

    public FacturaePage()
    {
        var app = (App)Application.Current;
        ViewModel = new FacturaePageViewModel(
            new WindowsFacePortalLauncherService(),
            app.FilePickerService,
            app.OperationSession);
        InitializeComponent();
        LocalValidationTitle.Text = T("paridad.lote3.invoice.title");
        ValidateInvoiceButton.Content = T("paridad.lote3.invoice.choose");
        ExportInvoiceReportButton.Content = T("paridad.lote3.invoice.export");
        InvoiceValidationSummary.Text = T("paridad.lote3.invoice.empty");
        InvoiceValidationReport.Header = T("paridad.lote3.invoice.report");
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

    private async void OnValidateInvoiceClick(object sender, RoutedEventArgs args)
    {
        var app = (App)Application.Current;
        var path = await app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument);
        if (string.IsNullOrWhiteSpace(path)) return;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ValidateInvoice, out var operations))
        {
            InvoiceValidationSummary.Text = T("paridad.lote3.invoice.unavailable");
            return;
        }
        ValidateInvoiceButton.IsEnabled = false;
        InvoiceValidationSummary.Text = T("paridad.lote3.invoice.validating");
        ExportInvoiceReportButton.IsEnabled = false;
        _invoiceReport = string.Empty;
        try
        {
            var result = await operations.ValidateInvoiceAsync(path);
            if (result.IsSuccess && result.Data is not null)
            {
                var data = result.Data;
                _invoiceReport = data.Report;
                InvoiceValidationReport.Text = data.Report;
                InvoiceValidationSummary.Text = (data.Valid ? T("paridad.lote3.invoice.valid") : T("paridad.lote3.invoice.invalid")) +
                    " " + T("paridad.lote3.invoice.summary").Replace("%1", data.Format).Replace("%2", data.Errors.ToString(CultureInfo.CurrentCulture)).Replace("%3", data.Warnings.ToString(CultureInfo.CurrentCulture));
                ExportInvoiceReportButton.IsEnabled = true;
            }
            else InvoiceValidationSummary.Text = T("paridad.lote3.invoice.failed").Replace("%1", result.SafeUserMessage);
        }
        catch (Exception)
        {
            InvoiceValidationSummary.Text = T("paridad.lote3.invoice.failed").Replace("%1", T("paridad.lote3.invoice.retry"));
        }
        finally { ValidateInvoiceButton.IsEnabled = true; }
    }

    private async void OnExportInvoiceReportClick(object sender, RoutedEventArgs args)
    {
        if (_invoiceReport.Length == 0) return;
        var app = (App)Application.Current;
        var saved = await app.FilePickerService.PickAndSaveTextFileAsync(SaveFilePickerProfile.InvoiceReport, _invoiceReport);
        InvoiceValidationSummary.Text = saved ? T("paridad.lote3.invoice.exported") : T("paridad.lote3.invoice.export_cancelled");
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
