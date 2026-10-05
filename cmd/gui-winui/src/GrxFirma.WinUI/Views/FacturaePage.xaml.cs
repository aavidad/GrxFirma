// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.Core.Ipc;
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
    private string _verifactuReport = string.Empty;
    private string _verifactuQrUrl = string.Empty;
    private bool _verifactuBusy;
    // Igual que el motor: el QR tributario va en la factura, normalmente en la primera página.
    private const int MaximumQrPdfPages = 5;
    // Código estable del motor (IPC errorCode) para «QR no encontrado».
    private const string VeriFactuQrNotFoundCode = "verifactu_qr_not_found";
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
        VeriFactuSummary.Text = T("verifactu.empty_state");
        VeriFactuReport.Header = T("verifactu.report");
        ExportVeriFactuReportButton.Content = T("verifactu.export");
        VeriFactuQrInput.Header = T("verifactu.qr_url_field");
        VeriFactuQrInput.PlaceholderText = T("verifactu.qr_url_label");
        VeriFactuQrQueryHelp.Text = T("verifactu.qr_query_help");
        VeriFactuQrTechnical.Header = T("verifactu.qr_technical");
        VeriFactuQrNotice.Text = T("verifactu.qr_notice");
        ReadVeriFactuQrButton.Content = T("verifactu.qr_read");
        ReadVeriFactuQrFileButton.Content = T("verifactu.qr_from_file");
        QueryVeriFactuQrButton.Content = T("verifactu.qr_query");
        VeriFactuQrReport.Header = T("verifactu.qr_result");
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
        ExportVeriFactuReportButton.IsEnabled = !busy && _verifactuReport.Length > 0;
        VeriFactuQrInput.IsEnabled = !busy;
        ReadVeriFactuQrButton.IsEnabled = !busy;
        ReadVeriFactuQrFileButton.IsEnabled = !busy;
        QueryVeriFactuQrButton.IsEnabled = !busy && _verifactuQrUrl.Length > 0;
    }

    private async Task ValidateVeriFactuAsync(bool folder)
    {
        if (_verifactuBusy) return;
        var app = (App)Application.Current;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ValidateVeriFactu, out var operations))
        { VeriFactuSummary.Text = T("paridad.lote3.invoice.unavailable"); return; }
        SetVeriFactuBusy(true);
        try
        {
            var path = folder ? await app.FilePickerService.PickFolderAsync()
                : await app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument);
            if (string.IsNullOrWhiteSpace(path)) return;
            // El resultado se muestra junto a los botones de Veri*Factu.
            _verifactuReport = string.Empty;
            VeriFactuReport.Text = string.Empty;
            VeriFactuReport.Visibility = Visibility.Collapsed;
            ExportVeriFactuReportButton.Visibility = Visibility.Collapsed;
            VeriFactuSummary.Text = T("paridad.lote3.invoice.validating");
            var result = await operations.ValidateVeriFactuAsync(path);
            if (result.IsSuccess && result.Data is not null)
            {
                _verifactuReport = result.Data.Report;
                VeriFactuReport.Text = _verifactuReport;
                VeriFactuReport.Visibility = Visibility.Visible;
                ExportVeriFactuReportButton.Visibility = Visibility.Visible;
                // El motor resume con avisos y errores: «sin errores» a secas
                // solo cuando no hay ninguno de los dos.
                VeriFactuSummary.Text = result.Data.Summary.Length > 0
                    ? result.Data.Summary
                    : T(result.Data.Valid ? "verifactu.valid" : "verifactu.invalid");
            }
            else VeriFactuSummary.Text = result.SafeUserMessage;
        }
        catch (Exception) { VeriFactuSummary.Text = T("verifactu.input"); }
        finally { SetVeriFactuBusy(false); }
    }

    private void OnVeriFactuQrTextChanged(object sender, TextChangedEventArgs args)
    {
        // TextChanged llega después de que la página escriba la URL leída de
        // una imagen: esa URL ya validada no borra el resultado.
        if (_verifactuQrUrl.Length > 0 &&
            string.Equals(VeriFactuQrInput.Text, _verifactuQrUrl, StringComparison.Ordinal)) return;
        if (_verifactuBusy) return;
        _verifactuQrUrl = string.Empty;
        if (QueryVeriFactuQrButton is not null) QueryVeriFactuQrButton.IsEnabled = false;
        if (VeriFactuQrReport is not null) VeriFactuQrReport.Text = string.Empty;
        HideVeriFactuQrTechnical();
    }

    private async void OnReadVeriFactuQrClick(object sender, RoutedEventArgs args)
    {
        if (_verifactuBusy) return;
        var app = (App)Application.Current;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ReadVeriFactuQr, out var operations)) return;
        var text = VeriFactuQrInput.Text.Trim();
        if (text.Length == 0)
        {
            VeriFactuQrReport.Text = T("verifactu.qr_empty");
            VeriFactuQrInput.Focus(FocusState.Programmatic);
            return;
        }
        SetVeriFactuBusy(true);
        _verifactuQrUrl = string.Empty;
        try
        {
            var result = await operations.ReadVeriFactuQrAsync(text);
            ShowVeriFactuQrResult(result);
        }
        // El cliente rechaza antes de enviar lo que no es una URL de cotejo de la AEAT.
        catch (ArgumentException) { VeriFactuQrReport.Text = T("verifactu.qr_url"); }
        catch (Exception) { VeriFactuQrReport.Text = T("verifactu.qr_params"); }
        finally { SetVeriFactuBusy(false); }
    }

    // Muestra los cuatro datos del QR y habilita el cotejo explícito solo con
    // la URL que devuelve el motor tras validarla.
    private void HideVeriFactuQrTechnical()
    {
        if (VeriFactuQrTechnical is null || VeriFactuQrTechnicalText is null) return;
        VeriFactuQrTechnical.IsExpanded = false;
        VeriFactuQrTechnical.Visibility = Visibility.Collapsed;
        VeriFactuQrTechnicalText.Text = string.Empty;
    }

    private void ShowVeriFactuQrResult(IpcCallResult<VeriFactuQrResult> result)
    {
        HideVeriFactuQrTechnical();
        if (result.IsSuccess && result.Data is not null)
        {
            var qr = result.Data;
            _verifactuQrUrl = qr.Url;
            // La URL leída de una imagen o un PDF queda a la vista y se puede copiar.
            if (!string.Equals(VeriFactuQrInput.Text, qr.Url, StringComparison.Ordinal))
                VeriFactuQrInput.Text = qr.Url;
            var culture = GrxFirma.WinUI.Core.Localization.AppCulture.For(Localizer.Language);
            VeriFactuQrReport.Text = T("verifactu.qr_nif") + ": " + qr.Nif + "\n" +
                T("verifactu.qr_number") + ": " + qr.Number + "\n" +
                T("verifactu.qr_date") + ": " + VeriFactuQrDisplay.Date(qr.Date, culture) + "\n" +
                T("verifactu.qr_amount") + ": " + VeriFactuQrDisplay.Amount(qr.Amount, culture);
        }
        else VeriFactuQrReport.Text = result.SafeUserMessage;
    }

    private async void OnReadVeriFactuQrFileClick(object sender, RoutedEventArgs args)
    {
        if (_verifactuBusy) return;
        var app = (App)Application.Current;
        if (!app.OperationSession.TryGetOperations(DesktopOperationActions.ReadVeriFactuQr, out var operations)) return;
        SetVeriFactuBusy(true);
        try
        {
            var path = await app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.VeriFactuQrSource);
            if (string.IsNullOrWhiteSpace(path)) return;
            // Vaciar la URL limpia el informe; después se indica qué fichero se lee.
            VeriFactuQrInput.Text = string.Empty;
            _verifactuQrUrl = string.Empty;
            VeriFactuQrReport.Text = T("verifactu.qr_reading");
            // VeriFactuQrInput es también el nombre del cuadro de texto de esta página.
            if (!global::GrxFirma.WinUI.Core.Operations.VeriFactuQrInput.IsPdfSource(path))
            {
                ShowVeriFactuQrResult(await operations.ReadVeriFactuQrFromFileAsync(path));
                return;
            }
            // El motor de Windows no rasteriza PDF: se hace aquí con
            // Windows.Data.Pdf y se envía cada página como PNG acotado.
            var preview = new WindowsPdfPreviewService();
            IpcCallResult<VeriFactuQrResult>? last = null;
            var total = 1;
            for (var page = 1; page <= total && page <= MaximumQrPdfPages; page++)
            {
                PdfPreviewResult rendered;
                try { rendered = await preview.RenderPageForCodeReadingAsync(path, page, CancellationToken.None); }
                // Si la página a 2 000 px supera el tope seguro, se usa la resolución de la vista previa.
                catch (InvalidDataException) { rendered = await preview.RenderPageAsync(path, page, CancellationToken.None); }
                total = rendered.TotalPages;
                last = await operations.ReadVeriFactuQrFromImageAsync(rendered.Data);
                if (last.IsSuccess) break;
                // Solo «no encontrado» justifica probar la página siguiente: un
                // plazo vencido, otra búsqueda en curso o cualquier otro fallo
                // se muestran ya, sin lanzar más escaneos sobre el motor.
                if (!string.Equals(last.ErrorCode, VeriFactuQrNotFoundCode, StringComparison.Ordinal)) break;
            }
            if (last is null) VeriFactuQrReport.Text = T("verifactu.qr_pdf");
            else ShowVeriFactuQrResult(last);
        }
        catch (Exception) { VeriFactuQrReport.Text = T("verifactu.qr_pdf"); }
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
            HideVeriFactuQrTechnical();
            if (result.IsSuccess && result.Data is not null)
            {
                // Una frase para la persona; el JSON queda tras «Ver respuesta técnica».
                var raw = result.Data.Response.ToString();
                VeriFactuQrReport.Text = T(VeriFactuQrResponse.Classify(raw));
                VeriFactuQrTechnicalText.Text = raw;
                VeriFactuQrTechnical.Visibility = Visibility.Visible;
            }
            else VeriFactuQrReport.Text = result.SafeUserMessage;
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

    private async void OnExportVeriFactuReportClick(object sender, RoutedEventArgs args)
    {
        if (_verifactuReport.Length == 0) return;
        var app = (App)Application.Current;
        var saved = await app.FilePickerService.PickAndSaveTextFileAsync(SaveFilePickerProfile.InvoiceReport, _verifactuReport);
        VeriFactuSummary.Text = saved ? T("paridad.lote3.invoice.exported") : T("paridad.lote3.invoice.export_cancelled");
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
