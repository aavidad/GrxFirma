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
    private string _verifactuQrUrl = string.Empty;
    private bool _verifactuBusy;
    private static string T(string key) => SealUiCatalog.Text(Localizer.Language, key);

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
        VeriFactuTitle.Text = T("verifactu.title");
        VeriFactuScope.Text = T("verifactu.scope");
        ValidateVeriFactuButton.Content = T("verifactu.choose");
        ValidateVeriFactuFolderButton.Content = T("verifactu.folder");
        VeriFactuQrTitle.Text = T("verifactu.qr_title");
        VeriFactuQrInput.Header = T("verifactu.qr_url_label");
        VeriFactuQrNotice.Text = T("verifactu.qr_notice");
        ReadVeriFactuQrButton.Content = T("verifactu.qr_read");
        QueryVeriFactuQrButton.Content = T("verifactu.qr_query");
        VeriFactuQrReport.Header = T("verifactu.qr_title");
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
        if (_verifactuBusy) return;
        var app = (App)Application.Current;
        var path = await app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument);
        if (string.IsNullOrWhiteSpace(path)) return;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ValidateInvoice, out var operations))
        {
            InvoiceValidationSummary.Text = T("paridad.lote3.invoice.unavailable");
            return;
        }
        SetVeriFactuBusy(true);
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
        finally { SetVeriFactuBusy(false); }
    }

    private async void OnValidateVeriFactuClick(object sender, RoutedEventArgs args) => await ValidateVeriFactuAsync(false);
    private async void OnValidateVeriFactuFolderClick(object sender, RoutedEventArgs args) => await ValidateVeriFactuAsync(true);

    private void SetVeriFactuBusy(bool busy)
    {
        _verifactuBusy = busy;
        ValidateInvoiceButton.IsEnabled = !busy;
        ValidateVeriFactuButton.IsEnabled = !busy;
        ValidateVeriFactuFolderButton.IsEnabled = !busy;
        VeriFactuQrInput.IsEnabled = !busy;
        ReadVeriFactuQrButton.IsEnabled = !busy;
        QueryVeriFactuQrButton.IsEnabled = !busy && _verifactuQrUrl.Length > 0;
    }

    private async Task ValidateVeriFactuAsync(bool folder)
    {
        if (_verifactuBusy) return;
        var app = (App)Application.Current;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ValidateVeriFactu, out var operations))
        { InvoiceValidationSummary.Text = T("paridad.lote3.invoice.unavailable"); return; }
        SetVeriFactuBusy(true);
        try
        {
            var path = folder ? await app.FilePickerService.PickFolderAsync()
                : await app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument);
            if (string.IsNullOrWhiteSpace(path)) return;
            ExportInvoiceReportButton.IsEnabled = false;
            _invoiceReport = string.Empty;
            InvoiceValidationSummary.Text = T("paridad.lote3.invoice.validating");
            var result = await operations.ValidateVeriFactuAsync(path);
            if (result.IsSuccess && result.Data is not null)
            {
                _invoiceReport = result.Data.Report;
                InvoiceValidationReport.Text = _invoiceReport;
                InvoiceValidationSummary.Text = T(result.Data.Valid ? "verifactu.valid" : "verifactu.invalid");
                ExportInvoiceReportButton.IsEnabled = true;
            }
            else InvoiceValidationSummary.Text = result.SafeUserMessage;
        }
        catch (Exception) { InvoiceValidationSummary.Text = T("verifactu.input"); }
        finally { SetVeriFactuBusy(false); }
    }

    private void OnVeriFactuQrTextChanged(object sender, TextChangedEventArgs args)
    {
        _verifactuQrUrl = string.Empty;
        if (QueryVeriFactuQrButton is not null) QueryVeriFactuQrButton.IsEnabled = false;
        if (VeriFactuQrReport is not null) VeriFactuQrReport.Text = string.Empty;
    }

    private async void OnReadVeriFactuQrClick(object sender, RoutedEventArgs args)
    {
        if (_verifactuBusy) return;
        var app = (App)Application.Current;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ReadVeriFactuQr, out var operations)) return;
        SetVeriFactuBusy(true);
        _verifactuQrUrl = string.Empty;
        try
        {
            var result = await operations.ReadVeriFactuQrAsync(VeriFactuQrInput.Text);
            if (result.IsSuccess && result.Data is not null)
            {
                var qr = result.Data;
                _verifactuQrUrl = qr.Url;
                VeriFactuQrReport.Text = T("verifactu.qr_nif") + ": " + qr.Nif + "\n" +
                    T("verifactu.qr_number") + ": " + qr.Number + "\n" +
                    T("verifactu.qr_date") + ": " + qr.Date + "\n" +
                    T("verifactu.qr_amount") + ": " + qr.Amount;
            }
            else VeriFactuQrReport.Text = result.SafeUserMessage;
        }
        catch (Exception) { VeriFactuQrReport.Text = T("verifactu.qr_params"); }
        finally { SetVeriFactuBusy(false); }
    }

    private async void OnQueryVeriFactuQrClick(object sender, RoutedEventArgs args)
    {
        if (_verifactuBusy || _verifactuQrUrl.Length == 0) return;
        var app = (App)Application.Current;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.QueryVeriFactuQr, out var operations)) return;
        SetVeriFactuBusy(true);
        try
        {
            var result = await operations.QueryVeriFactuQrAsync(_verifactuQrUrl);
            VeriFactuQrReport.Text = result.IsSuccess && result.Data is not null
                ? result.Data.Response.ToString() : result.SafeUserMessage;
        }
        catch (Exception) { VeriFactuQrReport.Text = T("verifactu.qr_service"); }
        finally { SetVeriFactuBusy(false); }
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
