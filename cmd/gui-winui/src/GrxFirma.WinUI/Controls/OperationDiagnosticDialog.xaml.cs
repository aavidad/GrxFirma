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

        _diagnostic = ContextualFailureAssistant.ForDisplay(diagnostic);
        _filePicker = filePicker;
        ViewModel = new OperationDiagnosticDialogViewModel(_diagnostic);
        InitializeComponent();
        ContextualOperation.Text = Localizer.LabelValue(
            Localizer.Text("winui.firmar.confirmar.operacion"),
            Localizer.Text(ContextualFailureAssistant.OperationKey(_diagnostic.Action)));
        var failedStep = ViewModel.Presentation.Steps.LastOrDefault(step =>
            step.Status == DiagnosticStepStatus.Failure);
        ContextualStep.Text = Localizer.LabelValue(Localizer.Text("winui.parity.support.current"),
            failedStep?.Label ?? Localizer.Text("report.status.unknown"));
        ContextualNextAction.Text = Localizer.Text(ContextualFailureAssistant.ActionKey(_diagnostic));
        ContextualEvidence.ItemsSource = ContextualFailureAssistant.Rows(_diagnostic);
        Closed += (_, _) =>
        {
            _closed = true;
            _localCheckCancellation?.Cancel();
            if (_correctFieldRequested) _correctFieldAction?.Invoke();
        };
    }

    private CancellationTokenSource? _localCheckCancellation;
    private bool _closed;

    private async void OnCheckLocal(object sender, RoutedEventArgs args)
    {
        if (_localCheckCancellation is not null || Application.Current is not App app) return;
        using var cancellation = new CancellationTokenSource(TimeSpan.FromSeconds(15));
        _localCheckCancellation = cancellation;
        CheckLocalButton.IsEnabled = false;
        LocalCheckStatus.Text = Localizer.Text("winui.diagnostico.diagnostico_en_curso");
        try
        {
            var evidence = await ContextualFailureAssistant.CheckLocalAsync(app.OperationSession, cancellation.Token);
            if (!_closed) ContextualEvidence.ItemsSource = ContextualFailureAssistant.Rows(_diagnostic, evidence);
        }
        catch (OperationCanceledException)
        {
            // Al cerrar no se actualizan controles del diálogo ni se abre otro.
        }
        finally
        {
            _localCheckCancellation = null;
            if (!_closed)
            {
                CheckLocalButton.IsEnabled = true;
                LocalCheckStatus.Text = Localizer.Text("winui.l27.assistant.checked");
            }
        }
    }

    private readonly OperationDiagnostic _diagnostic;
    private readonly IFilePickerService _filePicker;

    public Action? CorrectFieldAction
    {
        get => _correctFieldAction;
        set
        {
            _correctFieldAction = value;
            SecondaryButtonText = value is null ? string.Empty : Localizer.Text("validacion.corregir");
        }
    }

    private Action? _correctFieldAction;
    private bool _correctFieldRequested;

    public OperationDiagnosticDialogViewModel ViewModel { get; }

    private void OnCorrectField(ContentDialog sender, ContentDialogButtonClickEventArgs args)
    {
        _correctFieldRequested = true;
    }

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
                Localizer.Text("winui.diagnostico.diagnostico_grxfirma") +
                $"{generatedAtUtc:yyyyMMddTHHmmssZ}");

            ExportStatusBar.Severity = saved
                ? InfoBarSeverity.Success
                : InfoBarSeverity.Informational;
            ExportStatusBar.Title = saved
                ? Localizer.Text("winui.diagnostico.informe_guardado")
                : Localizer.Text("winui.diagnostico.exportacion_cancelada");
            ExportStatusBar.Message = saved
                ? Localizer.Text("winui.diagnostico.el_informe_saneado_esta_listo_para")
                : Localizer.Text("winui.diagnostico.no_se_ha_guardado_ningun_informe");
        }
        catch
        {
            ExportStatusBar.Severity = InfoBarSeverity.Error;
            ExportStatusBar.Title = Localizer.Text("winui.diagnostico.no_se_pudo_guardar_el_informe");
            ExportStatusBar.Message =
                Localizer.Text("winui.diagnostico.intentelo_de_nuevo_en_otra_ubicacion");
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
                Localizer.Text("winui.diagnostico.el_selector_de_informes_todavia_no_esta"));
}
