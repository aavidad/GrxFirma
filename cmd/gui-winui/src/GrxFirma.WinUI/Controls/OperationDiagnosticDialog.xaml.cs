// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

public sealed partial class OperationDiagnosticDialog : ContentDialog
{
    public OperationDiagnosticDialog(OperationDiagnostic diagnostic)
        : this(diagnostic, ResolveFilePickerService())
    {
    }

    internal OperationDiagnosticDialog(
        OperationDiagnostic diagnostic,
        IFilePickerService filePicker)
    {
        ArgumentNullException.ThrowIfNull(diagnostic);
        ArgumentNullException.ThrowIfNull(filePicker);

        _diagnostic = diagnostic;
        _filePicker = filePicker;
        ViewModel = new OperationDiagnosticDialogViewModel(diagnostic);
        InitializeComponent();
    }

    private readonly OperationDiagnostic _diagnostic;
    private readonly IFilePickerService _filePicker;

    public OperationDiagnosticDialogViewModel ViewModel { get; }

    private async void OnExportReport(
        ContentDialog sender,
        ContentDialogButtonClickEventArgs args)
    {
        args.Cancel = true;
        var deferral = args.GetDeferral();
        IsPrimaryButtonEnabled = false;
        ExportStatusBar.IsOpen = false;
        try
        {
            var generatedAtUtc = DateTimeOffset.UtcNow;
            var report = DiagnosticIncidentReport.CreateJson(
                _diagnostic,
                generatedAtUtc);
            var saved = await _filePicker.PickAndSaveTextFileAsync(
                SaveFilePickerProfile.DiagnosticReport,
                report,
                $"diagnostico-grxfirma-{generatedAtUtc:yyyyMMddTHHmmssZ}");

            ExportStatusBar.Severity = saved
                ? InfoBarSeverity.Success
                : InfoBarSeverity.Informational;
            ExportStatusBar.Title = saved
                ? "Informe guardado"
                : "Exportación cancelada";
            ExportStatusBar.Message = saved
                ? "El informe saneado está listo para entregarlo a soporte."
                : "No se ha guardado ningún informe.";
        }
        catch
        {
            ExportStatusBar.Severity = InfoBarSeverity.Error;
            ExportStatusBar.Title = "No se pudo guardar el informe";
            ExportStatusBar.Message =
                "Inténtelo de nuevo en otra ubicación.";
        }
        finally
        {
            ExportStatusBar.IsOpen = true;
            IsPrimaryButtonEnabled = true;
            deferral.Complete();
        }
    }

    private static IFilePickerService ResolveFilePickerService() =>
        Application.Current is App app &&
        app.FilePickerService is not null
            ? app.FilePickerService
            : throw new InvalidOperationException(
                "El selector de informes todavía no está disponible.");
}
