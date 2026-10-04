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

public sealed class DiagnosticsPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumVisibleSteps = 12;
    private const string LocalChannelCode = "local_channel";
    private const string EnginePingCode = "engine_ping";
    private const string LocalClockCode = "local_clock";
    private const string RemoteClockCode = "remote_clock";
    private const string CertificatesCode = "certificate_inventory";
    private const string CertificateAccessCode =
        "certificate_access_inventory";
    private const string ProxyStoreCode = "proxy_secure_store";
    private const string TlsStoreCode = "tls_local_store";
    private const string GovernmentAFirmaCode = "government_afirma";

    private readonly DesktopOperationSession _session;
    private IReadOnlyList<DiagnosticStepPresentation> _steps = [];
    private CancellationTokenSource? _operationCancellation;
    private OperationDiagnostic? _currentDiagnostic;
    private bool _isActive;
    private bool _isBusy;
    private bool _canRun;
    private bool _canCancel;
    private bool _canOpenDetailedDiagnostic;
    private bool _hasResult;
    private string _summaryTitle = "Sin comprobaciones";
    private string _summaryMessage =
        "Pulse «Ejecutar diagnóstico» para comprobar únicamente las fases que publica el motor local.";
    private string _ownerLabel = "Indeterminado";
    private string _responsibility =
        "Todavía no se ha observado ningún fallo.";
    private string _suggestedAction =
        "Ejecute el diagnóstico cuando el motor local esté conectado.";
    private string _tlsStatusTitle = "Estado TLS pendiente";
    private string _tlsStatusMessage =
        "Ejecute el diagnóstico para inspeccionar el material TLS local gestionado.";
    private string _certificateSummary =
        "El resumen local de certificados todavía no se ha consultado.";
    private InfoBarSeverity _resultSeverity =
        InfoBarSeverity.Informational;
    private InfoBarSeverity _tlsStatusSeverity =
        InfoBarSeverity.Informational;
    private bool _hasTlsStatus;
    private bool _canRefreshTlsStatus;
    private bool _canInstallTlsTrust;
    private bool _canClearTlsTrust;
    private bool _hasManagedTlsMaterial;
    private int _operationInProgress;
    private string _currentProbeCode = EnginePingCode;

    public DiagnosticsPageViewModel(DesktopOperationSession session)
        : base(
            "Diagnóstico",
            "Comprueba paso a paso el canal, el reloj y los inventarios disponibles, sin repetir una firma ni aceptar endpoints remotos arbitrarios.",
            "El diagnóstico no está disponible porque el motor local no ha publicado ping.")
    {
        ArgumentNullException.ThrowIfNull(session);
        _session = session;
        PublishInitialTimeline();
        RefreshAvailability();
    }

    public event Action<OperationDiagnostic>? DiagnosticRequested;

    public IReadOnlyList<DiagnosticStepPresentation> Steps
    {
        get => _steps;
        private set => SetProperty(ref _steps, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanRun
    {
        get => _canRun;
        private set => SetProperty(ref _canRun, value);
    }

    public bool CanCancel
    {
        get => _canCancel;
        private set => SetProperty(ref _canCancel, value);
    }

    public bool CanOpenDetailedDiagnostic
    {
        get => _canOpenDetailedDiagnostic;
        private set => SetProperty(
            ref _canOpenDetailedDiagnostic,
            value);
    }

    public bool HasResult
    {
        get => _hasResult;
        private set => SetProperty(ref _hasResult, value);
    }

    public string SummaryTitle
    {
        get => _summaryTitle;
        private set => SetProperty(ref _summaryTitle, Localizer.Text(value));
    }

    public string SummaryMessage
    {
        get => _summaryMessage;
        private set => SetProperty(ref _summaryMessage, Localizer.Text(value));
    }

    public string OwnerLabel
    {
        get => _ownerLabel;
        private set => SetProperty(ref _ownerLabel, Localizer.Text(value));
    }

    public string Responsibility
    {
        get => _responsibility;
        private set => SetProperty(ref _responsibility, Localizer.Text(value));
    }

    public string SuggestedAction
    {
        get => _suggestedAction;
        private set => SetProperty(ref _suggestedAction, Localizer.Text(value));
    }

    public InfoBarSeverity ResultSeverity
    {
        get => _resultSeverity;
        private set => SetProperty(ref _resultSeverity, value);
    }

    public string TlsStatusTitle
    {
        get => _tlsStatusTitle;
        private set => SetProperty(ref _tlsStatusTitle, Localizer.Text(value));
    }

    public string TlsStatusMessage
    {
        get => _tlsStatusMessage;
        private set => SetProperty(ref _tlsStatusMessage, Localizer.Text(value));
    }

    public string CertificateSummary
    {
        get => _certificateSummary;
        private set => SetProperty(ref _certificateSummary, Localizer.Text(value));
    }

    public InfoBarSeverity TlsStatusSeverity
    {
        get => _tlsStatusSeverity;
        private set => SetProperty(ref _tlsStatusSeverity, value);
    }

    public bool HasTlsStatus
    {
        get => _hasTlsStatus;
        private set => SetProperty(ref _hasTlsStatus, value);
    }

    public bool CanRefreshTlsStatus
    {
        get => _canRefreshTlsStatus;
        private set => SetProperty(ref _canRefreshTlsStatus, value);
    }

    public bool CanInstallTlsTrust
    {
        get => _canInstallTlsTrust;
        private set => SetProperty(ref _canInstallTlsTrust, value);
    }

    public bool CanClearTlsTrust
    {
        get => _canClearTlsTrust;
        private set => SetProperty(ref _canClearTlsTrust, value);
    }

    public void Activate()
    {
        if (_isActive)
        {
            return;
        }

        _isActive = true;
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
        CancelCurrentOperation();
        RefreshAvailability();
    }

    public void CancelCurrentOperation()
    {
        try
        {
            Volatile.Read(ref _operationCancellation)?.Cancel();
        }
        catch (ObjectDisposedException)
        {
            // La operación ya terminó y liberó su token.
        }
    }

    public void RequestDetailedDiagnostic()
    {
        if (_currentDiagnostic is not null && !IsBusy)
        {
            DiagnosticRequested?.Invoke(_currentDiagnostic);
        }
    }

    public async Task RunDiagnosticAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            return;
        }

        var observedSteps = CreateInitialSteps().ToList();
        var failures = new List<OperationDiagnostic>();
        _currentDiagnostic = null;
        HasResult = true;
        SummaryTitle = "Diagnóstico en curso";
        SummaryMessage =
            "Se están ejecutando comprobaciones acotadas de solo lectura.";
        OwnerLabel = "Aún no determinado";
        Responsibility =
            "Espere a que terminen las fases disponibles.";
        SuggestedAction =
            "Puede cancelar sin modificar certificados ni configuración.";
        ResultSeverity = InfoBarSeverity.Informational;
        PublishSteps(observedSteps);

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.Ping,
                out var operations))
            {
                AddLocalFailure(
                    observedSteps,
                    failures,
                    LocalChannelCode,
                    "Canal seguro con el motor local",
                    "El canal local dejó de estar disponible antes de iniciar las comprobaciones.",
                    "Cierre la aplicación, ábrala desde el lanzador y vuelva a intentarlo.",
                    "IPC_UNAVAILABLE");
                MarkPendingLocalStepsAsSkipped(observedSteps);
                FinishRun(
                    observedSteps,
                    failures,
                    cancelled: false);
                return;
            }

            ReplaceStep(
                observedSteps,
                LocalChannelCode,
                Step(
                    LocalChannelCode,
                    "Canal seguro con el motor local",
                    DiagnosticStepStatus.Success,
                    "app_local",
                    "La interfaz ya completó el saludo autenticado del protocolo local.",
                    "Continúe con las comprobaciones del motor.",
                    evidenceRef: "phase:admission"));
            PublishSteps(observedSteps);

            _currentProbeCode = EnginePingCode;
            var ping = await operations.PingAsync(
                operationCancellation.Token);
            if (!IsSuccessful(ping))
            {
                AddMappedFailure(
                    observedSteps,
                    failures,
                    EnginePingCode,
                    "Respuesta del motor local",
                    OperationDiagnosticMapper.FromResult(ping));
                MarkPendingLocalStepsAsSkipped(observedSteps);
                FinishRun(observedSteps, failures, cancelled: false);
                return;
            }
            ReplaceStep(
                observedSteps,
                EnginePingCode,
                Step(
                    EnginePingCode,
                    "Respuesta del motor local",
                    DiagnosticStepStatus.Success,
                    "app_local",
                    "El motor respondió a una petición ping real.",
                    "Continúe con el inventario local.",
                    evidenceRef: "phase:operation"));
            PublishSteps(observedSteps);

            await ProbeClockDiagnosticsAsync(
                operations,
                observedSteps,
                failures,
                operationCancellation.Token);
            await ProbeCertificatesAsync(
                operations,
                observedSteps,
                failures,
                operationCancellation.Token);
            await ProbeCertificateAccessAsync(
                operations,
                observedSteps,
                failures,
                operationCancellation.Token);
            await ProbeProxyStoreAsync(
                operations,
                observedSteps,
                failures,
                operationCancellation.Token);
            await ProbeTlsStoreAsync(
                operations,
                observedSteps,
                failures,
                operationCancellation.Token);
            await ProbeDiagnosticSummaryAsync(
                operations,
                operationCancellation.Token);

            FinishRun(observedSteps, failures, cancelled: false);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            MarkPendingLocalStepsAsSkipped(observedSteps);
            FinishRun(observedSteps, failures, cancelled: true);
        }
        catch (Exception exception)
        {
            var diagnostic = OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
            AddMappedFailure(
                observedSteps,
                failures,
                _currentProbeCode,
                ProbeLabel(_currentProbeCode),
                diagnostic);
            MarkPendingLocalStepsAsSkipped(observedSteps);
            FinishRun(observedSteps, failures, cancelled: false);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    private async Task ProbeClockDiagnosticsAsync(
        DesktopOperationsClient operations,
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        CancellationToken cancellationToken)
    {
        _currentProbeCode = LocalClockCode;
        if (!_session.Supports(
            DesktopOperationActions.ClockDiagnostics))
        {
            ReplaceStep(
                steps,
                LocalClockCode,
                UnknownStep(
                    LocalClockCode,
                    "Reloj local",
                    "app_local",
                    "No comprobado: el motor conectado no publica el diagnóstico acotado de fecha y hora."));
            PublishSteps(steps);
            return;
        }

        var result = await operations.GetClockDiagnosticsAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            AddMappedFailure(
                steps,
                failures,
                LocalClockCode,
                "Diagnóstico de fecha y hora",
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is not { IsCoherent: true })
        {
            AddLocalFailure(
                steps,
                failures,
                LocalClockCode,
                "Diagnóstico de fecha y hora",
                "El motor devolvió fases de reloj incoherentes.",
                "Repare o actualice la instalación antes de atribuir el fallo a la hora del equipo.",
                "CLOCK_DIAGNOSTIC_INCOHERENT");
            return;
        }

        foreach (var remoteStep in result.Data.Steps)
        {
            ReplaceStep(
                steps,
                remoteStep.Code,
                remoteStep.ToOperationStep());
        }
        PublishSteps(steps);
    }

    private async Task ProbeCertificatesAsync(
        DesktopOperationsClient operations,
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        CancellationToken cancellationToken)
    {
        _currentProbeCode = CertificatesCode;
        if (!_session.Supports(
            DesktopOperationActions.CheckCertificates))
        {
            ReplaceStep(
                steps,
                CertificatesCode,
                Step(
                    CertificatesCode,
                    "Certificados utilizables",
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    "El motor conectado no publica el resumen check_certificates.",
                    "Actualice o repare la instalación para habilitar esta comprobación."));
            PublishSteps(steps);
            return;
        }

        var result = await operations.CheckCertificatesAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            AddMappedFailure(
                steps,
                failures,
                CertificatesCode,
                "Certificados utilizables",
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is null || !result.Data.HasCoherentCounts)
        {
            AddLocalFailure(
                steps,
                failures,
                CertificatesCode,
                "Certificados utilizables",
                "El motor no devolvió conteos de certificados coherentes.",
                "Abra Certificados y vuelva a cargar el almacén local.",
                "DIAGNOSTIC_CERTIFICATE_COUNTS");
            return;
        }

        var usable = result.Data.OkCount;
        var unavailable = result.Data.FailCount;
        if (usable <= 0)
        {
            AddLocalFailure(
                steps,
                failures,
                CertificatesCode,
                "Certificados utilizables",
                unavailable > 0
                    ? Localizer.Fill("Se encontraron {unavailable} certificado(s), pero ninguno está disponible para firmar.",
                        ("unavailable", unavailable.ToString()))
                    : "No se encontró ningún certificado disponible para firmar.",
                "Abra Certificados y revise el almacén, la caducidad y el dispositivo criptográfico.",
                "NO_USABLE_CERTIFICATE",
                owner: "certificate_store");
            return;
        }

        ReplaceStep(
            steps,
            CertificatesCode,
            Step(
                CertificatesCode,
                "Certificados utilizables",
                DiagnosticStepStatus.Success,
                "certificate_store",
                unavailable > 0
                    ? Localizer.Fill("Hay {usable} certificado(s) utilizable(s) y {unavailable} que requieren revisión.",
                        ("usable", usable.ToString()), ("unavailable", unavailable.ToString()))
                    : Localizer.Fill("Hay {usable} certificado(s) utilizable(s) para firmar.",
                        ("usable", usable.ToString())),
                "Seleccione un certificado utilizable al iniciar la firma.",
                evidenceRef: "phase:operation"));
        PublishSteps(steps);
    }

    private async Task ProbeCertificateAccessAsync(
        DesktopOperationsClient operations,
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        CancellationToken cancellationToken)
    {
        _currentProbeCode = CertificateAccessCode;
        if (!_session.Supports(
            DesktopOperationActions.CertificateAccessOptions))
        {
            ReplaceStep(
                steps,
                CertificateAccessCode,
                Step(
                    CertificateAccessCode,
                    "Acceso local a certificados",
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    "El motor no publica el inventario de gestores e importadores.",
                    "Use la página Certificados para revisar las opciones disponibles."));
            PublishSteps(steps);
            return;
        }

        var result = await operations.GetCertificateAccessOptionsAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            AddMappedFailure(
                steps,
                failures,
                CertificateAccessCode,
                "Acceso local a certificados",
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is null)
        {
            AddLocalFailure(
                steps,
                failures,
                CertificateAccessCode,
                "Acceso local a certificados",
                "El motor no devolvió un inventario de acceso a certificados.",
                "Repare la instalación si el problema se repite.",
                "MISSING_CERTIFICATE_ACCESS_INVENTORY");
            return;
        }

        var managers = result.Data.VisibleManagerCount;
        var targets = result.Data.VisibleImportTargetCount;
        if (managers == 0 && targets == 0)
        {
            ReplaceStep(
                steps,
                CertificateAccessCode,
                Step(
                    CertificateAccessCode,
                    "Acceso local a certificados",
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    "No se detectaron gestores ni destinos de importación; esto no demuestra por sí solo un fallo de firma.",
                    "Revise Certificados si necesita importar o administrar una credencial.",
                    evidenceRef: "phase:operation"));
            PublishSteps(steps);
            return;
        }

        ReplaceStep(
            steps,
            CertificateAccessCode,
            Step(
                CertificateAccessCode,
                "Acceso local a certificados",
                DiagnosticStepStatus.Success,
                "app_local",
                Localizer.Fill("El motor detectó {managers} gestor(es) y {targets} destino(s) de importación.",
                    ("managers", managers.ToString()), ("targets", targets.ToString())),
                "Use Certificados si necesita abrir un gestor o importar una credencial.",
                evidenceRef: "phase:operation"));
        PublishSteps(steps);
    }

    private async Task ProbeProxyStoreAsync(
        DesktopOperationsClient operations,
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        CancellationToken cancellationToken)
    {
        _currentProbeCode = ProxyStoreCode;
        if (!_session.Supports(
            DesktopOperationActions.ProxySecretStoreStatus))
        {
            ReplaceStep(
                steps,
                ProxyStoreCode,
                Step(
                    ProxyStoreCode,
                    "Almacén seguro del proxy",
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    "El motor no publica el inventario del almacén seguro del proxy.",
                    "No configure credenciales de proxy hasta disponer de un almacén seguro."));
            PublishSteps(steps);
            return;
        }

        var result = await operations.GetProxySecretStoreStatusAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            AddMappedFailure(
                steps,
                failures,
                ProxyStoreCode,
                "Almacén seguro del proxy",
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is null)
        {
            AddLocalFailure(
                steps,
                failures,
                ProxyStoreCode,
                "Almacén seguro del proxy",
                "El motor no devolvió el estado del almacén seguro.",
                "Revise la configuración del proxy o repare la instalación.",
                "MISSING_PROXY_STORE_STATUS",
                owner: "network_proxy");
            return;
        }

        var mode = result.Data.RuntimeMode.Trim().ToLowerInvariant();
        if (result.Data.Available)
        {
            ReplaceStep(
                steps,
                ProxyStoreCode,
                Step(
                    ProxyStoreCode,
                    "Almacén seguro del proxy",
                    DiagnosticStepStatus.Success,
                    "network_proxy",
                    "El motor confirmó que hay un almacén seguro disponible para credenciales de proxy.",
                    "No se muestran ni se leen credenciales durante este diagnóstico.",
                    evidenceRef: "phase:operation"));
        }
        else if (mode is "disabled" or "system" or
            "default-environment")
        {
            ReplaceStep(
                steps,
                ProxyStoreCode,
                Step(
                    ProxyStoreCode,
                    "Almacén seguro del proxy",
                    DiagnosticStepStatus.Skipped,
                    "network_proxy",
                    "La configuración actual no requiere credenciales guardadas en el almacén seguro.",
                    "No es necesario actuar mientras no use un proxy manual autenticado.",
                    evidenceRef: "phase:operation"));
        }
        else if (mode is "manual-no-secret" or "fail-closed")
        {
            AddLocalFailure(
                steps,
                failures,
                ProxyStoreCode,
                "Almacén seguro del proxy",
                "El proxy manual no dispone de credenciales utilizables en un almacén seguro.",
                "Abra Configuración y guarde de nuevo las credenciales del proxy.",
                "PROXY_SECURE_STORE_UNAVAILABLE",
                owner: "network_proxy");
            return;
        }
        else
        {
            ReplaceStep(
                steps,
                ProxyStoreCode,
                Step(
                    ProxyStoreCode,
                    "Almacén seguro del proxy",
                    DiagnosticStepStatus.Unknown,
                    "network_proxy",
                    "El inventario no permite determinar si el proxy necesita un almacén seguro.",
                    "Revise la configuración del proxy antes de atribuir un fallo remoto.",
                    evidenceRef: "phase:operation"));
        }
        PublishSteps(steps);
    }

    private async Task ProbeTlsStoreAsync(
        DesktopOperationsClient operations,
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        CancellationToken cancellationToken)
    {
        _currentProbeCode = TlsStoreCode;
        if (!_session.Supports(DesktopOperationActions.TlsDiagnostics))
        {
            ReplaceStep(
                steps,
                TlsStoreCode,
                Step(
                    TlsStoreCode,
                    "Material TLS local",
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    "El motor conectado no publica el inventario TLS local.",
                    "Repare o actualice la instalación para habilitar esta comprobación."));
            TlsStatusTitle = "Diagnóstico TLS no publicado";
            TlsStatusMessage =
                "Esta versión del motor no permite consultar el material TLS gestionado.";
            TlsStatusSeverity = InfoBarSeverity.Warning;
            HasTlsStatus = true;
            PublishSteps(steps);
            return;
        }

        var result = await operations.GetTlsDiagnosticsAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            var diagnostic = OperationDiagnosticMapper.FromResult(result);
            AddMappedFailure(
                steps,
                failures,
                TlsStoreCode,
                "Material TLS local",
                diagnostic);
            ApplyTlsFailure(diagnostic);
            return;
        }
        if (result.Data is not { IsCoherent: true })
        {
            AddLocalFailure(
                steps,
                failures,
                TlsStoreCode,
                "Material TLS local",
                "El motor devolvió un inventario TLS incoherente.",
                "Repare la instalación antes de modificar la confianza TLS.",
                "TLS_DIAGNOSTIC_INCOHERENT",
                owner: "app_local");
            ApplyTlsFailure(
                failures[^1]);
            return;
        }

        ApplyTlsStore(result.Data);
        var status = result.Data.State switch
        {
            "available" => DiagnosticStepStatus.Success,
            "unavailable" => DiagnosticStepStatus.Failure,
            _ => DiagnosticStepStatus.Skipped,
        };
        ReplaceStep(
            steps,
            TlsStoreCode,
            Step(
                TlsStoreCode,
                "Material TLS local",
                status,
                "app_local",
                TlsStatusMessage,
                status == DiagnosticStepStatus.Failure
                    ? "Use las acciones TLS de esta pantalla para reparar o retirar únicamente la confianza gestionada."
                    : "Instale la confianza local solo si necesita la integración con navegador.",
                evidenceRef: "phase:operation"));
        if (status == DiagnosticStepStatus.Failure)
        {
            failures.Add(new OperationDiagnostic
            {
                Category = "app_local",
                FailureCode = "TLS_STORE_UNAVAILABLE",
                UserMessage = TlsStatusMessage,
                ExpertMessage =
                    "El inventario tipado devolvió el estado unavailable sin exponer rutas locales.",
                LikelyOwner = "app_local",
                ResponsibilityMessage =
                    "El fallo observado pertenece al almacén TLS local.",
                SuggestedAction =
                    "Repare o retire la confianza TLS gestionada y vuelva a ejecutar el diagnóstico.",
                UserCanResolveDirectly = true,
            });
        }
        PublishSteps(steps);
    }

    private async Task ProbeDiagnosticSummaryAsync(
        DesktopOperationsClient operations,
        CancellationToken cancellationToken)
    {
        if (!_session.Supports(
            DesktopOperationActions.ExportDiagnostic))
        {
            CertificateSummary =
                "El motor no publica el resumen agregado de certificados.";
            return;
        }

        var result = await operations.ExportDiagnosticSummaryAsync(
            cancellationToken);
        if (!IsSuccessful(result) ||
            result.Data is not { IsCoherent: true })
        {
            CertificateSummary =
                "No se pudo confirmar un resumen coherente de certificados.";
            return;
        }

        CertificateSummary = Localizer.Fill(
            "Certificados detectados: {count}. Con acceso de firma confirmado: {signable}.",
            ("count", result.Data.CertificateCount.ToString()),
            ("signable", result.Data.CanSignCount.ToString()));
    }

    public async Task RefreshTlsStatusAsync()
    {
        if (!TryBeginStandaloneOperation(
                DesktopOperationActions.TlsDiagnostics,
                out var operations,
                out var operationCancellation))
        {
            UpdateCommandStates();
            return;
        }
        try
        {
            var result = await operations.GetTlsDiagnosticsAsync(
                operationCancellation.Token);
            if (!IsSuccessful(result) ||
                result.Data is not { IsCoherent: true })
            {
                ApplyTlsFailure(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            ApplyTlsStore(result.Data);
            await ProbeDiagnosticSummaryAsync(
                operations,
                operationCancellation.Token);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            TlsStatusTitle = "Comprobación TLS cancelada";
            TlsStatusMessage =
                "No se modificó la confianza ni el material TLS local.";
            TlsStatusSeverity = InfoBarSeverity.Warning;
            HasTlsStatus = true;
        }
        catch (Exception exception)
        {
            ApplyTlsFailure(
                OperationDiagnosticMapper.FromException(
                    exception,
                    operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task InstallTlsTrustAsync()
    {
        if (!TryBeginStandaloneOperation(
                DesktopOperationActions.InstallPublicRoots,
                out var operations,
                out var operationCancellation))
        {
            UpdateCommandStates();
            return;
        }
        try
        {
            var result = await operations.InstallPublicRootsAsync(
                operationCancellation.Token);
            if (!IsSuccessful(result))
            {
                ApplyTlsFailure(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            var refreshed = await operations.GetTlsDiagnosticsAsync(
                operationCancellation.Token);
            if (IsSuccessful(refreshed) &&
                refreshed.Data is { IsCoherent: true })
            {
                ApplyTlsStore(refreshed.Data);
            }
            TlsStatusTitle = "Confianza TLS local instalada";
            TlsStatusMessage =
                "El motor confirmó la CA local gestionada para el usuario actual. Solo se usa para la integración local con navegador.";
            TlsStatusSeverity = InfoBarSeverity.Success;
            HasTlsStatus = true;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            TlsStatusTitle = "Instalación TLS cancelada";
            TlsStatusMessage =
                "No se recibió confirmación de que la confianza quedara instalada.";
            TlsStatusSeverity = InfoBarSeverity.Warning;
            HasTlsStatus = true;
        }
        catch (Exception exception)
        {
            ApplyTlsFailure(
                OperationDiagnosticMapper.FromException(
                    exception,
                    operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task ClearTlsTrustAsync()
    {
        if (!TryBeginStandaloneOperation(
                DesktopOperationActions.ClearTlsTrust,
                out var operations,
                out var operationCancellation))
        {
            UpdateCommandStates();
            return;
        }
        try
        {
            var result = await operations.ClearTlsTrustAsync(
                operationCancellation.Token);
            if (!IsSuccessful(result) ||
                result.Data < 0)
            {
                ApplyTlsFailure(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            _hasManagedTlsMaterial = false;
            var refreshed = await operations.GetTlsDiagnosticsAsync(
                operationCancellation.Token);
            if (IsSuccessful(refreshed) &&
                refreshed.Data is { IsCoherent: true })
            {
                ApplyTlsStore(refreshed.Data);
            }
            TlsStatusTitle = "Confianza TLS local retirada";
            TlsStatusMessage = Localizer.Fill(
                "El motor confirmó la retirada de la confianza y de {count} artefacto(s) gestionado(s).",
                ("count", result.Data.ToString()));
            TlsStatusSeverity = InfoBarSeverity.Success;
            HasTlsStatus = true;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            TlsStatusTitle = "Retirada TLS cancelada";
            TlsStatusMessage =
                "No se recibió confirmación de que la confianza quedara retirada.";
            TlsStatusSeverity = InfoBarSeverity.Warning;
            HasTlsStatus = true;
        }
        catch (Exception exception)
        {
            ApplyTlsFailure(
                OperationDiagnosticMapper.FromException(
                    exception,
                    operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    private void ApplyTlsStore(TlsStoreDiagnosticResult store)
    {
        _hasManagedTlsMaterial =
            store.State is "available" or "unavailable";
        HasTlsStatus = true;
        switch (store.State)
        {
            case "available":
                TlsStatusTitle = "Material TLS local disponible";
                TlsStatusMessage = Localizer.Fill(
                    "Se inventariaron {artifacts} artefacto(s): {certificates} certificado(s) y {keys} clave(s). Este inventario no afirma por sí solo que el navegador confíe en la CA.",
                    ("artifacts", store.ArtifactCount.ToString()),
                    ("certificates", store.CertificateCount.ToString()),
                    ("keys", store.KeyCount.ToString()));
                TlsStatusSeverity = InfoBarSeverity.Success;
                break;
            case "empty":
                TlsStatusTitle = "Almacén TLS local vacío";
                TlsStatusMessage =
                    "No hay material TLS gestionado. Instale la confianza solo si necesita firmar desde el navegador.";
                TlsStatusSeverity = InfoBarSeverity.Warning;
                break;
            case "not_created":
                TlsStatusTitle = "Material TLS no creado";
                TlsStatusMessage =
                    "La integración local con navegador todavía no ha creado su CA ni sus claves.";
                TlsStatusSeverity = InfoBarSeverity.Informational;
                break;
            default:
                TlsStatusTitle = "Almacén TLS no disponible";
                TlsStatusMessage =
                    "El motor no pudo inspeccionar de forma segura el material TLS local.";
                TlsStatusSeverity = InfoBarSeverity.Error;
                break;
        }
        UpdateCommandStates();
    }

    private void ApplyTlsFailure(OperationDiagnostic diagnostic)
    {
        _currentDiagnostic = diagnostic;
        var presentation =
            OperationDiagnosticPresentation.From(diagnostic);
        TlsStatusTitle = "No se pudo completar la operación TLS";
        TlsStatusMessage = presentation.Cause;
        TlsStatusSeverity = InfoBarSeverity.Error;
        HasTlsStatus = true;
        UpdateCommandStates();
    }

    private void FinishRun(
        List<OperationDiagnosticStep> observedSteps,
        IReadOnlyList<OperationDiagnostic> failures,
        bool cancelled)
    {
        PublishSteps(observedSteps);
        _currentDiagnostic = BuildAggregateDiagnostic(
            observedSteps,
            failures,
            cancelled);
        var presentation =
            OperationDiagnosticPresentation.From(_currentDiagnostic);
        SummaryTitle = cancelled
            ? "Diagnóstico cancelado"
            : observedSteps.Any(step =>
                step.Status == DiagnosticStepStatus.Failure)
                ? "Se detectó un problema"
                : "Comprobaciones terminadas";
        SummaryMessage = presentation.Cause;
        OwnerLabel = presentation.OwnerLabel;
        Responsibility = presentation.Responsibility;
        SuggestedAction = presentation.SuggestedAction;
        ResultSeverity = cancelled
            ? InfoBarSeverity.Warning
            : observedSteps.Any(step =>
                step.Status == DiagnosticStepStatus.Failure)
                ? InfoBarSeverity.Error
                : InfoBarSeverity.Warning;
        HasResult = true;
        UpdateCommandStates();
    }

    private static OperationDiagnostic BuildAggregateDiagnostic(
        IReadOnlyList<OperationDiagnosticStep> observedSteps,
        IReadOnlyList<OperationDiagnostic> failures,
        bool cancelled)
    {
        var failedStep = observedSteps.FirstOrDefault(step =>
            step.Status == DiagnosticStepStatus.Failure);
        var owner = cancelled
            ? "unknown"
            : AggregateOwner(failedStep?.Owner);
        var failureCode = failures
            .Select(failure => failure.FailureCode)
            .FirstOrDefault(code => !string.IsNullOrWhiteSpace(code)) ??
            (failedStep is null
                ? null
                : SafeIpcText.Clean(
                    failedStep.Code,
                    64,
                    "DIAGNOSTIC_FAILURE")
                    .ToUpperInvariant());

        string cause;
        string responsibility;
        string action;
        if (cancelled)
        {
            cause =
                "El diagnóstico se canceló. Las fases pendientes no se ejecutaron.";
            responsibility =
                "La cancelación no se atribuye a ningún sistema.";
            action =
                "Ejecute de nuevo el diagnóstico si desea completar las fases locales.";
        }
        else if (failedStep is null)
        {
            var remoteClock = observedSteps.FirstOrDefault(step =>
                step.Code == RemoteClockCode);
            cause = remoteClock?.Status ==
                DiagnosticStepStatus.Success
                    ? "Las comprobaciones disponibles han terminado y la hora local coincide con la cabecera Date del origen HTTPS observado. @firma no se ha comprobado."
                    : "Las comprobaciones disponibles han terminado. La hora remota y @firma que no tengan evidencia siguen como no comprobadas.";
            responsibility =
                "No se ha observado un fallo atribuible, pero una comparación horaria no demuestra por sí sola el estado completo del trámite remoto.";
            action =
                "Si una firma real falla, abra el diagnóstico de esa operación para conservar la evidencia exacta de cada fase.";
        }
        else
        {
            (cause, responsibility, action) = owner switch
            {
                "app_local" => (
                    "Se ha detectado un problema en este equipo o en la aplicación local.",
                    "El fallo observado pertenece a una fase local.",
                    failedStep.SuggestedAction ??
                        "Revise la fase marcada con una ✕ y vuelva a intentarlo."),
                "remote_service" => (
                    "La evidencia recibida sitúa el problema en un servidor remoto.",
                    "El fallo observado no pertenece al canal local.",
                    failedStep.SuggestedAction ??
                        "Conserve el diagnóstico y contacte con el portal."),
                "@firma" => (
                    "La evidencia recibida sitúa el problema en la plataforma @firma.",
                    "El fallo observado ha sido atribuido por el motor a @firma.",
                    failedStep.SuggestedAction ??
                        "Conserve el diagnóstico y contacte con el soporte del trámite."),
                _ => (
                    "Se ha detectado un fallo, pero su origen no puede determinarse.",
                    "La evidencia disponible no permite distinguir entre un origen local y remoto.",
                    failedStep.SuggestedAction ??
                        "Conserve el diagnóstico y solicite soporte."),
            };
        }

        return new OperationDiagnostic
        {
            Category = owner,
            FailureCode = cancelled
                ? "USER_CANCELLED"
                : failureCode,
            UserMessage = cause,
            ExpertMessage =
                "El asistente ejecutó únicamente acciones IPC publicadas. La sonda de hora remota solo puede usar un origen HTTPS observado y autorizado; no acepta URLs por IPC, no envía cookies ni credenciales y no prueba @firma por descarte.",
            LikelyOwner = owner,
            ResponsibilityMessage = responsibility,
            SuggestedAction = action,
            UserCanResolveDirectly = owner == "app_local",
            Steps = observedSteps
                .Take(MaximumVisibleSteps)
                .ToArray(),
        };
    }

    private void AddMappedFailure(
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        string code,
        string label,
        OperationDiagnostic diagnostic)
    {
        failures.Add(diagnostic);
        var mappedStep = diagnostic.Steps.FirstOrDefault();
        ReplaceStep(
            steps,
            code,
            Step(
                code,
                label,
                DiagnosticStepStatus.Failure,
                AggregateOwner(
                    mappedStep?.Owner ??
                    diagnostic.LikelyOwner),
                diagnostic.UserMessage,
                diagnostic.SuggestedAction,
                evidenceRef: mappedStep?.EvidenceRef));
        PublishSteps(steps);
    }

    private void AddLocalFailure(
        List<OperationDiagnosticStep> steps,
        List<OperationDiagnostic> failures,
        string code,
        string label,
        string message,
        string action,
        string failureCode,
        string owner = "app_local")
    {
        var diagnostic = new OperationDiagnostic
        {
            Category = owner,
            FailureCode = failureCode,
            UserMessage = message,
            ExpertMessage =
                "El motor respondió, pero el resumen tipado no contenía un estado local utilizable.",
            LikelyOwner = owner,
            ResponsibilityMessage =
                "El resultado incoherente se ha observado en una fase local.",
            SuggestedAction = action,
            UserCanResolveDirectly = true,
            Steps =
            [
                Step(
                    failureCode,
                    label,
                    DiagnosticStepStatus.Failure,
                    owner,
                    message,
                    action,
                    evidenceRef: "phase:operation"),
            ],
        };
        AddMappedFailure(
            steps,
            failures,
            code,
            label,
            diagnostic);
    }

    private void MarkPendingLocalStepsAsSkipped(
        List<OperationDiagnosticStep> steps)
    {
        foreach (var code in new[]
        {
            EnginePingCode,
            LocalClockCode,
            CertificatesCode,
            CertificateAccessCode,
            ProxyStoreCode,
            TlsStoreCode,
        })
        {
            var current = steps.First(step => step.Code == code);
            if (current.Status != DiagnosticStepStatus.Unknown)
            {
                continue;
            }
            ReplaceStep(
                steps,
                code,
                Step(
                    code,
                    ProbeLabel(code),
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    "La fase no se ejecutó porque el diagnóstico se detuvo antes.",
                    "Ejecute de nuevo el diagnóstico para completar esta fase."));
        }
    }

    private void PublishInitialTimeline()
    {
        Steps = CreateInitialSteps()
            .Select(DiagnosticStepPresentation.From)
            .ToArray();
    }

    private void PublishSteps(
        IReadOnlyList<OperationDiagnosticStep> steps)
    {
        Steps = steps
            .Take(MaximumVisibleSteps)
            .Select(DiagnosticStepPresentation.From)
            .ToArray();
    }

    private static IReadOnlyList<OperationDiagnosticStep>
        CreateInitialSteps() =>
    [
        UnknownStep(
            LocalChannelCode,
            "Canal seguro con el motor local",
            "app_local",
            "Se comprobará al iniciar el asistente."),
        UnknownStep(
            EnginePingCode,
            "Respuesta del motor local",
            "app_local",
            "Pendiente de una petición ping real."),
        UnknownStep(
            LocalClockCode,
            "Reloj local",
            "app_local",
            "Pendiente de una lectura real y del estado no destructivo del servicio de hora."),
        UnknownStep(
            CertificatesCode,
            "Certificados utilizables",
            "certificate_store",
            "Pendiente del resumen local de certificados."),
        UnknownStep(
            CertificateAccessCode,
            "Acceso local a certificados",
            "app_local",
            "Pendiente del inventario de gestores e importadores."),
        UnknownStep(
            ProxyStoreCode,
            "Almacén seguro del proxy",
            "network_proxy",
            "Pendiente del inventario seguro del proxy."),
        UnknownStep(
            TlsStoreCode,
            "Material TLS local",
            "app_local",
            "Pendiente del inventario TLS gestionado."),
        UnknownStep(
            RemoteClockCode,
            "Hora del servidor remoto observado",
            "remote_service",
            "No comprobado: todavía no hay un origen HTTPS observado y autorizado disponible para esta sonda."),
        UnknownStep(
            GovernmentAFirmaCode,
            "Plataforma @firma",
            "@firma",
            "No comprobado: ninguna acción publicada prueba @firma de forma aislada."),
    ];

    private static OperationDiagnosticStep UnknownStep(
        string code,
        string label,
        string owner,
        string message) =>
        Step(
            code,
            label,
            DiagnosticStepStatus.Unknown,
            owner,
            message,
            "Abra el diagnóstico de una operación real para obtener evidencia de esta fase.");

    private static OperationDiagnosticStep Step(
        string code,
        string label,
        DiagnosticStepStatus status,
        string owner,
        string message,
        string action,
        string? evidenceRef = null) =>
        new()
        {
            Code = SafeIpcText.Clean(code, 64, "unknown"),
            Label = SafeIpcText.Clean(label, 160, "Paso observado"),
            Status = status,
            Owner = owner,
            UserMessage = SafeIpcText.Clean(
                message,
                512,
                "Sin detalle adicional."),
            SuggestedAction = SafeIpcText.Clean(
                action,
                512,
                "No hay una acción específica registrada."),
            EvidenceRef = evidenceRef,
        };

    private static void ReplaceStep(
        IList<OperationDiagnosticStep> steps,
        string code,
        OperationDiagnosticStep replacement)
    {
        for (var index = 0; index < steps.Count; index++)
        {
            if (steps[index].Code == code)
            {
                steps[index] = replacement;
                return;
            }
        }
    }

    private static bool IsSuccessful<TData>(
        IpcCallResult<TData> result) =>
        result.IsSuccess &&
        string.Equals(
            result.Outcome,
            "success",
            StringComparison.Ordinal);

    private static string AggregateOwner(string? owner) =>
        OperationDiagnosticPresentation.MapOwner(owner) switch
        {
            DiagnosticOwner.Computer or
            DiagnosticOwner.Browser => "app_local",
            DiagnosticOwner.Portal => "remote_service",
            DiagnosticOwner.GovernmentAFirma => "@firma",
            _ => "unknown",
        };

    private static string ProbeLabel(string code) => code switch
    {
        LocalChannelCode => "Canal seguro con el motor local",
        EnginePingCode => "Respuesta del motor local",
        LocalClockCode => "Diagnóstico de fecha y hora",
        CertificatesCode => "Certificados utilizables",
        CertificateAccessCode => "Acceso local a certificados",
        ProxyStoreCode => "Almacén seguro del proxy",
        TlsStoreCode => "Material TLS local",
        _ => "Comprobación local",
    };

    private bool TryBeginOperation(
        CancellationToken cancellationToken,
        out CancellationTokenSource operationCancellation)
    {
        if (!_isActive ||
            !CanRun ||
            Interlocked.CompareExchange(
                ref _operationInProgress,
                1,
                0) != 0)
        {
            operationCancellation = null!;
            return false;
        }

        operationCancellation =
            CancellationTokenSource.CreateLinkedTokenSource(
                cancellationToken);
        Volatile.Write(
            ref _operationCancellation,
            operationCancellation);
        SetBusy(true);
        return true;
    }

    private bool TryBeginStandaloneOperation(
        string requiredAction,
        out DesktopOperationsClient operations,
        out CancellationTokenSource operationCancellation)
    {
        operations = null!;
        operationCancellation = null!;
        if (!_isActive ||
            IsBusy)
        {
            return false;
        }
        if (!_session.TryGetOperations(
                requiredAction,
                out var resolvedOperations))
        {
            return false;
        }
        if (Interlocked.CompareExchange(
                ref _operationInProgress,
                1,
                0) != 0)
        {
            return false;
        }

        operations = resolvedOperations;
        operationCancellation = new CancellationTokenSource();
        Volatile.Write(
            ref _operationCancellation,
            operationCancellation);
        SetBusy(true);
        return true;
    }

    private void EndOperation(
        CancellationTokenSource operationCancellation)
    {
        Interlocked.CompareExchange(
            ref _operationCancellation,
            null,
            operationCancellation);
        operationCancellation.Dispose();
        Interlocked.Exchange(ref _operationInProgress, 0);
        SetBusy(false);
    }

    private void OnAvailabilityChanged(object? sender, EventArgs args) =>
        RefreshAvailability();

    private void RefreshAvailability()
    {
        var available =
            _isActive &&
            _session.Supports(DesktopOperationActions.Ping);
        SetOperationAvailability(
            available,
            "Motor local conectado. El asistente ejecutará únicamente comprobaciones acotadas de solo lectura.");
        if (!available)
        {
            CancelCurrentOperation();
        }
        UpdateCommandStates();
    }

    private void SetBusy(bool value)
    {
        IsBusy = value;
        UpdateCommandStates();
    }

    private void UpdateCommandStates()
    {
        CanRun =
            _isActive &&
            IsOperationConnected &&
            !IsBusy;
        CanCancel = _isActive && IsBusy;
        CanOpenDetailedDiagnostic =
            _isActive &&
            !IsBusy &&
            _currentDiagnostic is not null;
        CanRefreshTlsStatus =
            _isActive &&
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.TlsDiagnostics);
        CanInstallTlsTrust =
            _isActive &&
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.InstallPublicRoots);
        CanClearTlsTrust =
            _isActive &&
            IsOperationConnected &&
            !IsBusy &&
            _hasManagedTlsMaterial &&
            _session.Supports(
                DesktopOperationActions.ClearTlsTrust);
    }
}
