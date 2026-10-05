// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed class VerifyPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumVisibleItems = 32;

    private readonly DesktopOperationSession _session;
    private readonly IFilePickerService _filePicker;
    private CancellationTokenSource? _pageLifetime;
    private CancellationTokenSource? _operationCancellation;
    private string? _signedFilePath;
    private string? _originalFilePath;
    private VerifyResult? _reportResult;
    private bool _hasHtmlReport;
    private string _signedBySummary = string.Empty;
    private bool _canExportReport;
    private string _signedFileName = string.Empty;
    private string _originalFileName = string.Empty;
    private string _resultTitle = Localizer.Text("winui.comun.sin_resultado");
    private string _resultMessage =
        Localizer.Text("winui.verificar.no_se_presupone_la_validez_de_ninguna");
    private string _integritySummary = Localizer.Text("winui.verificar.integridad_sin_datos");
    private string _certificateSummary = Localizer.Text("winui.verificar.certificado_sin_datos");
    private string _trustSummary = Localizer.Text("winui.verificar.confianza_sin_datos");
    private string _formatSummary = Localizer.Text("winui.verificar.formato_y_cobertura_sin_datos");
    private IReadOnlyList<string> _signers = [];
    private IReadOnlyList<string> _warnings = [];
    private IReadOnlyList<string> _details = [];
    private bool _isActive;
    private bool _isBusy;
    private bool _canSelectFiles;
    private bool _canVerify;
    private bool _canCancel;
    private bool _hasResult;
    private InfoBarSeverity _resultSeverity = InfoBarSeverity.Informational;

    public VerifyPageViewModel(
        DesktopOperationSession session,
        IFilePickerService filePicker)
        : base(
            Localizer.Text("winui.comun.verificar"),
            Localizer.Text("winui.verificar.comprueba_la_integridad_los_firmantes_la"),
            Localizer.Text("winui.verificar.la_verificacion_no_esta_disponible"))
    {
        ArgumentNullException.ThrowIfNull(session);
        ArgumentNullException.ThrowIfNull(filePicker);
        _session = session;
        _filePicker = filePicker;
    }

    public event Action<OperationDiagnostic>? DiagnosticRequested;

    public string SignedFileName
    {
        // Vacío sin selección: el PlaceholderText traducido muestra el aviso
        // y sigue el idioma aunque cambie en caliente.
        get => _signedFileName;
        private set => SetProperty(ref _signedFileName, value);
    }

    public string OriginalFileName
    {
        get => string.IsNullOrEmpty(_originalFileName) ? Localizer.Text("winui.verificar.no_seleccionado") : _originalFileName;
        private set => SetProperty(ref _originalFileName, value);
    }

    public string ResultTitle
    {
        get => _resultTitle;
        private set => SetProperty(ref _resultTitle, value);
    }

    public string ResultMessage
    {
        get => _resultMessage;
        private set => SetProperty(ref _resultMessage, value);
    }

    public string IntegritySummary
    {
        get => _integritySummary;
        private set => SetProperty(ref _integritySummary, value);
    }

    public string CertificateSummary
    {
        get => _certificateSummary;
        private set => SetProperty(ref _certificateSummary, value);
    }

    public string TrustSummary
    {
        get => _trustSummary;
        private set => SetProperty(ref _trustSummary, value);
    }

    public string FormatSummary
    {
        get => _formatSummary;
        private set => SetProperty(ref _formatSummary, value);
    }

    public IReadOnlyList<string> Signers
    {
        get => _signers;
        private set => SetProperty(ref _signers, value);
    }

    public IReadOnlyList<string> Warnings
    {
        get => _warnings;
        private set => SetProperty(ref _warnings, value);
    }

    public IReadOnlyList<string> Details
    {
        get => _details;
        private set => SetProperty(ref _details, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanSelectFiles
    {
        get => _canSelectFiles;
        private set => SetProperty(ref _canSelectFiles, value);
    }

    public bool CanVerify
    {
        get => _canVerify;
        private set => SetProperty(ref _canVerify, value);
    }

    public bool CanCancel
    {
        get => _canCancel;
        private set => SetProperty(ref _canCancel, value);
    }

    public bool HasResult
    {
        get => _hasResult;
        private set => SetProperty(ref _hasResult, value);
    }

    public bool CanExportReport
    {
        get => _canExportReport;
        private set => SetProperty(ref _canExportReport, value);
    }

    // Acción principal: el informe imprimible del motor. Si no llegó, el JSON.
    public bool HasHtmlReport
    {
        get => _hasHtmlReport;
        private set => SetProperty(ref _hasHtmlReport, value);
    }

    public string SignedBySummary
    {
        get => _signedBySummary;
        private set => SetProperty(ref _signedBySummary, value);
    }

    public async Task ExportReportAsync()
    {
        if (!CanExportReport || _reportResult is null || _pageLifetime is null) return;
        if (_reportResult.ReportHtml.Length == 0)
        {
            await ExportJsonReportAsync();
            return;
        }
        try
        {
            await _filePicker.PickAndSaveTextFileAsync(
                SaveFilePickerProfile.VerificationReportHtml, _reportResult.ReportHtml,
                null, _pageLifetime.Token);
        }
        catch (OperationCanceledException) when (_pageLifetime?.IsCancellationRequested == true) { }
        catch (Exception exception)
        {
            RequestDiagnostic(OperationDiagnosticMapper.FromException(exception));
        }
    }

    public async Task ExportJsonReportAsync()
    {
        if (!CanExportReport || _reportResult is null || _pageLifetime is null) return;
        try
        {
            var json = VerificationReport.Serialize(
                _reportResult, _signedFilePath, _originalFilePath, DateTimeOffset.UtcNow);
            await _filePicker.PickAndSaveTextFileAsync(
                SaveFilePickerProfile.VerificationReport, json,
                null, _pageLifetime.Token);
        }
        catch (OperationCanceledException) when (_pageLifetime?.IsCancellationRequested == true) { }
        catch (Exception exception)
        {
            RequestDiagnostic(OperationDiagnosticMapper.FromException(exception));
        }
    }

    public InfoBarSeverity ResultSeverity
    {
        get => _resultSeverity;
        private set => SetProperty(ref _resultSeverity, value);
    }

    public void Activate()
    {
        if (_isActive)
        {
            return;
        }

        _isActive = true;
        _pageLifetime = new CancellationTokenSource();
        _session.AvailabilityChanged += OnAvailabilityChanged;
        RefreshAvailability();
    }

    public void Deactivate()
    {
        if (!_isActive)
        {
            return;
        }

        _isActive = false;
        _session.AvailabilityChanged -= OnAvailabilityChanged;
        _operationCancellation?.Cancel();
        _pageLifetime?.Cancel();
        _pageLifetime?.Dispose();
        _pageLifetime = null;
        RefreshAvailability();
    }

    public async Task SelectSignedFileAsync()
    {
        if (!CanSelectFiles || _pageLifetime is null)
        {
            return;
        }

        try
        {
            var path = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.SignedOrOriginalDocument,
                _pageLifetime.Token);
            if (path is null)
            {
                return;
            }

            _signedFilePath = path;
            SignedFileName = DisplayFileName(path);
            ResetResult();
            RefreshCommandState();
        }
        catch (OperationCanceledException)
            when (_pageLifetime?.IsCancellationRequested == true)
        {
        }
        catch (Exception exception)
        {
            RequestDiagnostic(OperationDiagnosticMapper.FromException(exception));
        }
    }

    public async Task SelectOriginalFileAsync()
    {
        if (!CanSelectFiles || _pageLifetime is null)
        {
            return;
        }

        try
        {
            var path = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.SignableDocument,
                _pageLifetime.Token);
            if (path is null)
            {
                return;
            }

            _originalFilePath = path;
            OriginalFileName = DisplayFileName(path);
            ResetResult();
        }
        catch (OperationCanceledException)
            when (_pageLifetime?.IsCancellationRequested == true)
        {
        }
        catch (Exception exception)
        {
            RequestDiagnostic(OperationDiagnosticMapper.FromException(exception));
        }
    }

    public void ClearOriginalFile()
    {
        if (IsBusy)
        {
            return;
        }

        _originalFilePath = null;
        OriginalFileName = string.Empty;
        ResetResult();
    }

    public async Task VerifyAsync()
    {
        if (!CanVerify ||
            _signedFilePath is null ||
            !_session.TryGetOperations(
                DesktopOperationActions.Verify,
                out var operations) ||
            _pageLifetime is null)
        {
            RefreshAvailability();
            return;
        }

        using var operationCancellation =
            CancellationTokenSource.CreateLinkedTokenSource(
                _pageLifetime.Token);
        _operationCancellation = operationCancellation;
        SetBusy(true);
        ResetResult();
        ResultTitle = Localizer.Text("winui.verificar.verificando");
        ResultMessage =
            Localizer.Text("winui.verificar.el_motor_local_esta_comprobando_la_firma");

        try
        {
            var result = await operations.VerifyAsync(
                new VerifyParameters
                {
                    InputPath = _signedFilePath,
                    OriginalPath = _originalFilePath,
                    ReportLanguage = Localizer.Language,
                },
                operationCancellation.Token);
            if (!result.IsSuccess || result.Outcome != "success")
            {
                HasResult = true;
                ResultSeverity = InfoBarSeverity.Error;
                ResultTitle = Localizer.Text("winui.verificar.no_se_pudo_verificar");
                ResultMessage = result.SafeUserMessage;
                RequestDiagnostic(OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (!IsCoherent(result.Data))
            {
                HasResult = true;
                ResultSeverity = InfoBarSeverity.Error;
                ResultTitle = Localizer.Text("winui.comun.resultado_no_utilizable");
                ResultMessage =
                    Localizer.Text("winui.verificar.el_motor_local_no_devolvio_evidencias_de");
                RequestDiagnostic(OperationDiagnosticMapper.FromException(
                    new InvalidOperationException()));
                return;
            }

            PresentResult(result.Data!);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            HasResult = true;
            ResultSeverity = InfoBarSeverity.Warning;
            ResultTitle = Localizer.Text("winui.verificar.verificacion_cancelada");
            ResultMessage =
                Localizer.Text("winui.comun.la_operacion_se_detuvo_antes_de_obtener");
        }
        catch (Exception exception)
        {
            HasResult = true;
            ResultSeverity = InfoBarSeverity.Error;
            ResultTitle = Localizer.Text("winui.verificar.no_se_pudo_verificar");
            ResultMessage =
                Localizer.Text("winui.verificar.la_operacion_termino_sin_un_resultado_de");
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token));
        }
        finally
        {
            if (ReferenceEquals(_operationCancellation, operationCancellation))
            {
                _operationCancellation = null;
            }
            SetBusy(false);
        }
    }

    public void CancelCurrentOperation() =>
        _operationCancellation?.Cancel();

    private void OnAvailabilityChanged(object? sender, EventArgs args) =>
        RefreshAvailability();

    private void RefreshAvailability()
    {
        var available =
            _isActive &&
            _session.Supports(DesktopOperationActions.Verify);
        SetOperationAvailability(
            available,
            Localizer.Text("winui.verificar.motor_local_listo_para_verificar_firmas"));
        if (!available)
        {
            _operationCancellation?.Cancel();
        }
        RefreshCommandState();
    }

    private void SetBusy(bool value)
    {
        IsBusy = value;
        RefreshCommandState();
    }

    private void RefreshCommandState()
    {
        CanSelectFiles =
            _isActive &&
            IsOperationConnected &&
            !IsBusy;
        CanVerify =
            CanSelectFiles &&
            !string.IsNullOrWhiteSpace(_signedFilePath);
        CanCancel = _isActive && IsBusy;
    }

    private void PresentResult(VerifyResult data)
    {
        _reportResult = data;
        CanExportReport = true;
        HasHtmlReport = data.ReportHtml.Length > 0;
        var trustStatus = NormalizeAspectStatus(data.Trust.Status);
        var hasValidSignatureEvidence =
            VerificationAssessment.HasValidSignatureEvidence(data);
        var hasXmlCompatibility =
            VerificationAssessment.HasXmlCanonicalizationCompatibility(data);
        HasResult = true;
        ResultTitle = VerificationPresentation.GetTitle(data);
        ResultSeverity = hasXmlCompatibility
            ? InfoBarSeverity.Warning
            : !hasValidSignatureEvidence
                ? InfoBarSeverity.Error
                : VerificationAssessment.HasEstablishedTrust(data)
                    ? InfoBarSeverity.Success
                    : InfoBarSeverity.Warning;
        ResultMessage = SafeIpcText.Clean(
            data.Reason,
            512,
            hasXmlCompatibility
                ? Localizer.Text("winui.verificar.la_firma_solo_supera_comprobaciones_de")
                : hasValidSignatureEvidence
                    ? Localizer.Text("winui.verificar.el_motor_confirmo_la_integridad")
                    : Localizer.Text("winui.verificar.el_motor_indico_que_la_firma_no_es"));
        IntegritySummary = AspectSummary(Localizer.Text("winui.verificar.integridad"), data.Integrity);
        CertificateSummary = AspectSummary(
            Localizer.Text("winui.verificar.certificado"),
            data.Certificate,
            masculine: true);
        TrustSummary = AspectSummary(Localizer.Text("winui.verificar.confianza"), data.Trust);
        FormatSummary = Localizer.Format("winui.verificar.formato_cobertura",
            SafeIpcText.Clean(data.Format, 80,
                Localizer.Text("winui.comun.no_determinado")),
            Localizer.Text(CoverageLabel(data.Coverage)));

        Signers = data.VisibleSignerSummaries
            .Select(item => item.Subject)
            .Concat(data.VisibleSigners)
            .Where(static item => !string.IsNullOrWhiteSpace(item))
            .Distinct(StringComparer.Ordinal)
            .Take(MaximumVisibleItems)
            .ToArray();
        // Lo primero que busca quien verifica: quién firmó (el motor no da la hora).
        var names = Signers.Select(DistinguishedNameText.CommonName)
            .Distinct(StringComparer.Ordinal).ToArray();
        SignedBySummary = names.Length == 0
            ? string.Empty
            : Localizer.Fill("winui.verificar.firmado_por", ("subject", string.Join(", ", names)));

        var warnings = data.VisibleWarnings
            .Concat(data.VisibleErrors)
            .Where(static item => !string.IsNullOrWhiteSpace(item))
            .Take(MaximumVisibleItems)
            .ToList();
        if (trustStatus == "unknown")
        {
            warnings.Insert(
                0,
                Localizer.Text("winui.verificar.la_confianza_del_certificado_no_ha_sido"));
        }
        Warnings = warnings.Take(MaximumVisibleItems).ToArray();

        Details = data.VisibleDetails
            .Concat(data.Integrity.VisibleDetails)
            .Concat(data.Certificate.VisibleDetails)
            .Concat(data.Trust.VisibleDetails)
            .Concat(data.VisibleEvidence.Select(
                item => Localizer.Format("{0}: {1}",
                    Localizer.VisibleText(SafeIpcText.Clean(item.Type, 80,
                        Localizer.Text("winui.verificar.evidencia"))),
                    Localizer.VisibleText(SafeIpcText.Clean(item.Summary, 320,
                        Localizer.Text("winui.verificar.sin_detalle"))))))
            .Where(static item => !string.IsNullOrWhiteSpace(item))
            .Distinct(StringComparer.Ordinal)
            .Take(MaximumVisibleItems)
            .ToArray();
    }

    private void ResetResult()
    {
        _reportResult = null;
        CanExportReport = false;
        HasHtmlReport = false;
        SignedBySummary = string.Empty;
        HasResult = false;
        ResultSeverity = InfoBarSeverity.Informational;
        ResultTitle = Localizer.Text("winui.comun.sin_resultado");
        ResultMessage =
            Localizer.Text("winui.verificar.no_se_presupone_la_validez_de_ninguna");
        IntegritySummary = Localizer.Text("winui.verificar.integridad_sin_datos");
        CertificateSummary = Localizer.Text("winui.verificar.certificado_sin_datos");
        TrustSummary = Localizer.Text("winui.verificar.confianza_sin_datos");
        FormatSummary = Localizer.Text("winui.verificar.formato_y_cobertura_sin_datos");
        Signers = [];
        Warnings = [];
        Details = [];
    }

    private void RequestDiagnostic(OperationDiagnostic diagnostic) =>
        DiagnosticRequested?.Invoke(diagnostic);

    private static bool IsCoherent(VerifyResult? data) =>
        VerificationAssessment.IsCoherent(data);

    private static string NormalizeAspectStatus(string? value) =>
        value?.Trim().ToLowerInvariant() switch
        {
            "valid" => "valid",
            "invalid" => "invalid",
            "warning" => "warning",
            _ => "unknown",
        };

    // masculine: «Certificado: válido»; el resto de aspectos son femeninos.
    private static string AspectSummary(
        string label,
        VerifyAspect aspect,
        bool masculine = false)
    {
        var status = NormalizeAspectStatus(aspect.Status);
        var statusLabel = status switch
        {
            "valid" => Localizer.Text(masculine ? "winui.verificar.valido" : "winui.verificar.valida"),
            "invalid" => Localizer.Text(masculine ? "winui.verificar.no_valido" : "winui.verificar.no_valida"),
            "warning" => Localizer.Text("winui.verificar.con_avisos"),
            _ => Localizer.Text(masculine ? "winui.verificar.no_determinado" : "winui.verificar.no_determinada"),
        };
        var reason = SafeIpcText.Clean(aspect.Reason, 320, string.Empty);
        return string.IsNullOrEmpty(reason)
            ? Localizer.Format("{0}: {1}.", Localizer.Text(label),
                Localizer.Text(statusLabel))
            : Localizer.Format("{0}: {1}. {2}", Localizer.Text(label),
                Localizer.Text(statusLabel), Localizer.VisibleText(reason));
    }

    private static string CoverageLabel(string? coverage) =>
        coverage?.Trim().ToLowerInvariant() switch
        {
            "full" => Localizer.Text("winui.verificar.completa"),
            "partial" => Localizer.Text("winui.verificar.parcial"),
            _ => Localizer.Text("winui.verificar.no_determinada"),
        };

    private static string DisplayFileName(string path) =>
        SafeIpcText.Clean(
            Path.GetFileName(path),
            256,
            Localizer.Text("winui.comun.fichero_seleccionado"));
}
