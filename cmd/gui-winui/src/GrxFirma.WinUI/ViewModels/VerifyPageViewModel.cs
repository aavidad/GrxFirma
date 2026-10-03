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
    private bool _canExportReport;
    private string _signedFileName = "Ningún fichero seleccionado";
    private string _originalFileName = "No seleccionado";
    private string _resultTitle = "Sin resultado";
    private string _resultMessage =
        "No se presupone la validez de ninguna firma hasta recibir evidencias del motor.";
    private string _integritySummary = "Integridad: sin datos";
    private string _certificateSummary = "Certificado: sin datos";
    private string _trustSummary = "Confianza: sin datos";
    private string _formatSummary = "Formato y cobertura: sin datos";
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
            "Verificar",
            "Comprueba la integridad, los firmantes, la cobertura y la confianza de una firma.",
            "La verificación no está disponible porque el motor local no ha publicado la operación necesaria.")
    {
        ArgumentNullException.ThrowIfNull(session);
        ArgumentNullException.ThrowIfNull(filePicker);
        _session = session;
        _filePicker = filePicker;
    }

    public event Action<OperationDiagnostic>? DiagnosticRequested;

    public string SignedFileName
    {
        get => _signedFileName;
        private set => SetProperty(ref _signedFileName, value);
    }

    public string OriginalFileName
    {
        get => _originalFileName;
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

    public async Task ExportReportAsync()
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
        OriginalFileName = "No seleccionado";
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
        ResultTitle = "Verificando…";
        ResultMessage =
            "El motor local está comprobando la firma. No cierre la aplicación.";

        try
        {
            var result = await operations.VerifyAsync(
                new VerifyParameters
                {
                    InputPath = _signedFilePath,
                    OriginalPath = _originalFilePath,
                },
                operationCancellation.Token);
            if (!result.IsSuccess || result.Outcome != "success")
            {
                HasResult = true;
                ResultSeverity = InfoBarSeverity.Error;
                ResultTitle = "No se pudo verificar";
                ResultMessage = result.SafeUserMessage;
                RequestDiagnostic(OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (!IsCoherent(result.Data))
            {
                HasResult = true;
                ResultSeverity = InfoBarSeverity.Error;
                ResultTitle = "Resultado no utilizable";
                ResultMessage =
                    "El motor local no devolvió evidencias de verificación coherentes.";
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
            ResultTitle = "Verificación cancelada";
            ResultMessage =
                "La operación se detuvo antes de obtener un resultado.";
        }
        catch (Exception exception)
        {
            HasResult = true;
            ResultSeverity = InfoBarSeverity.Error;
            ResultTitle = "No se pudo verificar";
            ResultMessage =
                "La operación terminó sin un resultado de verificación.";
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
            "Motor local listo para verificar firmas.");
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
                ? "La firma solo supera comprobaciones de compatibilidad histórica; no se acredita su validez conforme a XMLDSig."
                : hasValidSignatureEvidence
                    ? "El motor confirmó la integridad criptográfica de la firma."
                    : "El motor indicó que la firma no es válida.");
        IntegritySummary = AspectSummary("Integridad", data.Integrity);
        CertificateSummary = AspectSummary(
            "Certificado",
            data.Certificate);
        TrustSummary = AspectSummary("Confianza", data.Trust);
        FormatSummary =
            $"Formato: {SafeIpcText.Clean(data.Format, 80, "no determinado")}. " +
            $"Cobertura: {CoverageLabel(data.Coverage)}.";

        Signers = data.VisibleSignerSummaries
            .Select(item => item.Subject)
            .Concat(data.VisibleSigners)
            .Where(static item => !string.IsNullOrWhiteSpace(item))
            .Distinct(StringComparer.Ordinal)
            .Take(MaximumVisibleItems)
            .ToArray();

        var warnings = data.VisibleWarnings
            .Concat(data.VisibleErrors)
            .Where(static item => !string.IsNullOrWhiteSpace(item))
            .Take(MaximumVisibleItems)
            .ToList();
        if (trustStatus == "unknown")
        {
            warnings.Insert(
                0,
                "La confianza del certificado no ha sido determinada; no equivale a una identidad de confianza.");
        }
        Warnings = warnings.Take(MaximumVisibleItems).ToArray();

        Details = data.VisibleDetails
            .Concat(data.Integrity.VisibleDetails)
            .Concat(data.Certificate.VisibleDetails)
            .Concat(data.Trust.VisibleDetails)
            .Concat(data.VisibleEvidence.Select(
                item =>
                    $"{SafeIpcText.Clean(item.Type, 80, "Evidencia")}: " +
                    SafeIpcText.Clean(item.Summary, 320, "sin detalle")))
            .Where(static item => !string.IsNullOrWhiteSpace(item))
            .Distinct(StringComparer.Ordinal)
            .Take(MaximumVisibleItems)
            .ToArray();
    }

    private void ResetResult()
    {
        _reportResult = null;
        CanExportReport = false;
        HasResult = false;
        ResultSeverity = InfoBarSeverity.Informational;
        ResultTitle = "Sin resultado";
        ResultMessage =
            "No se presupone la validez de ninguna firma hasta recibir evidencias del motor.";
        IntegritySummary = "Integridad: sin datos";
        CertificateSummary = "Certificado: sin datos";
        TrustSummary = "Confianza: sin datos";
        FormatSummary = "Formato y cobertura: sin datos";
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

    private static string AspectSummary(
        string label,
        VerifyAspect aspect)
    {
        var status = NormalizeAspectStatus(aspect.Status);
        var statusLabel = status switch
        {
            "valid" => "válida",
            "invalid" => "no válida",
            "warning" => "con avisos",
            _ => "no determinada",
        };
        var reason = SafeIpcText.Clean(aspect.Reason, 320, string.Empty);
        return string.IsNullOrEmpty(reason)
            ? $"{label}: {statusLabel}."
            : $"{label}: {statusLabel}. {reason}";
    }

    private static string CoverageLabel(string? coverage) =>
        coverage?.Trim().ToLowerInvariant() switch
        {
            "full" => "completa",
            "partial" => "parcial",
            _ => "no determinada",
        };

    private static string DisplayFileName(string path) =>
        SafeIpcText.Clean(
            Path.GetFileName(path),
            256,
            "Fichero seleccionado");
}
