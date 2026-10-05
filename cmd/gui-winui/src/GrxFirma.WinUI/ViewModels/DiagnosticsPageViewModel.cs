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
    private string _summaryTitle = Localizer.Text("winui.comun.sin_comprobaciones");
    private string _summaryMessage =
        Localizer.Text("winui.diagnostico.pulse_ejecutar_diagnostico_para");
    private string _ownerLabel = Localizer.Text("winui.diagnostico.indeterminado");
    private string _responsibility =
        Localizer.Text("winui.diagnostico.todavia_no_se_ha_observado_ningun_fallo");
    private string _suggestedAction =
        Localizer.Text("winui.diagnostico.ejecute_el_diagnostico_cuando_el_motor");
    private string _tlsStatusTitle = Localizer.Text("winui.diagnostico.estado_tls_pendiente");
    private string _tlsStatusMessage =
        Localizer.Text("winui.diagnostico.ejecute_el_diagnostico_para_inspeccionar");
    private string _certificateSummary =
        Localizer.Text("winui.diagnostico.el_resumen_local_de_certificados_todavia");
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
            Localizer.Text("winui.comun.diagnostico"),
            Localizer.Text("winui.diagnostico.comprueba_paso_a_paso_el_canal_el_reloj"),
            Localizer.Text("winui.diagnostico.el_diagnostico_no_esta_disponible_porque"))
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
        SummaryTitle = Localizer.Text("winui.diagnostico.diagnostico_en_curso");
        SummaryMessage =
            Localizer.Text("winui.diagnostico.se_estan_ejecutando_comprobaciones");
        OwnerLabel = Localizer.Text("winui.diagnostico.aun_no_determinado");
        Responsibility =
            Localizer.Text("winui.diagnostico.espere_a_que_terminen_las_fases");
        SuggestedAction =
            Localizer.Text("winui.diagnostico.puede_cancelar_sin_modificar");
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
                    Localizer.Text("winui.diagnostico.canal_seguro_con_el_motor_local"),
                    Localizer.Text("winui.diagnostico.el_canal_local_dejo_de_estar_disponible"),
                    Localizer.Text("winui.diagnostico.cierre_la_aplicacion_abrala_desde_el"),
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
                    Localizer.Text("winui.diagnostico.canal_seguro_con_el_motor_local"),
                    DiagnosticStepStatus.Success,
                    "app_local",
                    Localizer.Text("winui.diagnostico.la_interfaz_ya_completo_el_saludo"),
                    Localizer.Text("winui.diagnostico.continue_con_las_comprobaciones_del"),
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
                    Localizer.Text("winui.diagnostico.respuesta_del_motor_local"),
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
                    Localizer.Text("winui.diagnostico.respuesta_del_motor_local"),
                    DiagnosticStepStatus.Success,
                    "app_local",
                    Localizer.Text("winui.diagnostico.el_motor_respondio_a_una_peticion_ping"),
                    Localizer.Text("winui.diagnostico.continue_con_el_inventario_local"),
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
                    Localizer.Text("winui.diagnostico.reloj_local"),
                    "app_local",
                    Localizer.Text("winui.diagnostico.no_comprobado_el_motor_conectado_no")));
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
                Localizer.Text("winui.diagnostico.diagnostico_de_fecha_y_hora"),
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is not { IsCoherent: true })
        {
            AddLocalFailure(
                steps,
                failures,
                LocalClockCode,
                Localizer.Text("winui.diagnostico.diagnostico_de_fecha_y_hora"),
                Localizer.Text("winui.diagnostico.el_motor_devolvio_fases_de_reloj"),
                Localizer.Text("winui.diagnostico.repare_o_actualice_la_instalacion_antes"),
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
                    Localizer.Text("winui.diagnostico.certificados_utilizables"),
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    Localizer.Text("winui.diagnostico.el_motor_conectado_no_publica_el_resumen"),
                    Localizer.Text("winui.diagnostico.actualice_o_repare_la_instalacion_para")));
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
                Localizer.Text("winui.diagnostico.certificados_utilizables"),
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is null || !result.Data.HasCoherentCounts)
        {
            AddLocalFailure(
                steps,
                failures,
                CertificatesCode,
                Localizer.Text("winui.diagnostico.certificados_utilizables"),
                Localizer.Text("winui.diagnostico.el_motor_no_devolvio_conteos_de"),
                Localizer.Text("winui.diagnostico.abra_certificados_y_vuelva_a_cargar_el"),
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
                Localizer.Text("winui.diagnostico.certificados_utilizables"),
                unavailable > 0
                    ? Localizer.Fill("winui.diagnostico.se_encontraron_certificado_s_pero",
                        ("unavailable", unavailable.ToString()))
                    : Localizer.Text("winui.diagnostico.no_se_encontro_ningun_certificado"),
                Localizer.Text("winui.diagnostico.abra_certificados_y_revise_el_almacen_la"),
                "NO_USABLE_CERTIFICATE",
                owner: "certificate_store");
            return;
        }

        ReplaceStep(
            steps,
            CertificatesCode,
            Step(
                CertificatesCode,
                Localizer.Text("winui.diagnostico.certificados_utilizables"),
                DiagnosticStepStatus.Success,
                "certificate_store",
                unavailable > 0
                    ? Localizer.Fill("winui.diagnostico.hay_certificado_s_utilizable_s_y_que",
                        ("usable", usable.ToString()), ("unavailable", unavailable.ToString()))
                    : Localizer.Fill("winui.diagnostico.hay_certificado_s_utilizable_s_para",
                        ("usable", usable.ToString())),
                Localizer.Text("winui.diagnostico.seleccione_un_certificado_utilizable_al"),
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
                    Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    Localizer.Text("winui.diagnostico.el_motor_no_publica_el_inventario_de"),
                    Localizer.Text("winui.diagnostico.use_la_pagina_certificados_para_revisar")));
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
                Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is null)
        {
            AddLocalFailure(
                steps,
                failures,
                CertificateAccessCode,
                Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
                Localizer.Text("winui.diagnostico.el_motor_no_devolvio_un_inventario_de"),
                Localizer.Text("winui.diagnostico.repare_la_instalacion_si_el_problema_se"),
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
                    Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    Localizer.Text("winui.diagnostico.no_se_detectaron_gestores_ni_destinos_de"),
                    Localizer.Text("winui.diagnostico.revise_certificados_si_necesita_importar"),
                    evidenceRef: "phase:operation"));
            PublishSteps(steps);
            return;
        }

        ReplaceStep(
            steps,
            CertificateAccessCode,
            Step(
                CertificateAccessCode,
                Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
                DiagnosticStepStatus.Success,
                "app_local",
                Localizer.Fill("winui.diagnostico.el_motor_detecto_gestor_es_y_destino_s",
                    ("managers", managers.ToString()), ("targets", targets.ToString())),
                Localizer.Text("winui.diagnostico.use_certificados_si_necesita_abrir_un"),
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
                    Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    Localizer.Text("winui.diagnostico.el_motor_no_publica_el_inventario_del"),
                    Localizer.Text("winui.diagnostico.no_configure_credenciales_de_proxy_hasta")));
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
                Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (result.Data is null)
        {
            AddLocalFailure(
                steps,
                failures,
                ProxyStoreCode,
                Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                Localizer.Text("winui.diagnostico.el_motor_no_devolvio_el_estado_del"),
                Localizer.Text("winui.diagnostico.revise_la_configuracion_del_proxy_o"),
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
                    Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                    DiagnosticStepStatus.Success,
                    "network_proxy",
                    Localizer.Text("winui.diagnostico.el_motor_confirmo_que_hay_un_almacen"),
                    Localizer.Text("winui.diagnostico.no_se_muestran_ni_se_leen_credenciales"),
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
                    Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                    DiagnosticStepStatus.Skipped,
                    "network_proxy",
                    Localizer.Text("winui.diagnostico.la_configuracion_actual_no_requiere"),
                    Localizer.Text("winui.diagnostico.no_es_necesario_actuar_mientras_no_use"),
                    evidenceRef: "phase:operation"));
        }
        else if (mode is "manual-no-secret" or "fail-closed")
        {
            AddLocalFailure(
                steps,
                failures,
                ProxyStoreCode,
                Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                Localizer.Text("winui.diagnostico.el_proxy_manual_no_dispone_de"),
                Localizer.Text("winui.diagnostico.abra_configuracion_y_guarde_de_nuevo_las"),
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
                    Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
                    DiagnosticStepStatus.Unknown,
                    "network_proxy",
                    Localizer.Text("winui.diagnostico.el_inventario_no_permite_determinar_si"),
                    Localizer.Text("winui.diagnostico.revise_la_configuracion_del_proxy_antes"),
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
                    Localizer.Text("winui.diagnostico.material_tls_local"),
                    DiagnosticStepStatus.Skipped,
                    "app_local",
                    Localizer.Text("winui.diagnostico.el_motor_conectado_no_publica_el"),
                    Localizer.Text("winui.diagnostico.repare_o_actualice_la_instalacion_para")));
            TlsStatusTitle = Localizer.Text("winui.diagnostico.diagnostico_tls_no_publicado");
            TlsStatusMessage =
                Localizer.Text("winui.diagnostico.esta_version_del_motor_no_permite");
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
                Localizer.Text("winui.diagnostico.material_tls_local"),
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
                Localizer.Text("winui.diagnostico.material_tls_local"),
                Localizer.Text("winui.diagnostico.el_motor_devolvio_un_inventario_tls"),
                Localizer.Text("winui.diagnostico.repare_la_instalacion_antes_de_modificar"),
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
                Localizer.Text("winui.diagnostico.material_tls_local"),
                status,
                "app_local",
                TlsStatusMessage,
                status == DiagnosticStepStatus.Failure
                    ? Localizer.Text("winui.diagnostico.use_las_acciones_tls_de_esta_pantalla")
                    : Localizer.Text("winui.diagnostico.instale_la_confianza_local_solo_si"),
                evidenceRef: "phase:operation"));
        if (status == DiagnosticStepStatus.Failure)
        {
            failures.Add(new OperationDiagnostic
            {
                Category = "app_local",
                FailureCode = "TLS_STORE_UNAVAILABLE",
                UserMessage = TlsStatusMessage,
                ExpertMessage =
                    Localizer.Text("winui.diagnostico.el_inventario_tipado_devolvio_el_estado"),
                LikelyOwner = "app_local",
                ResponsibilityMessage =
                    Localizer.Text("winui.diagnostico.el_fallo_observado_pertenece_al_almacen"),
                SuggestedAction =
                    Localizer.Text("winui.diagnostico.repare_o_retire_la_confianza_tls"),
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
                Localizer.Text("winui.diagnostico.el_motor_no_publica_el_resumen_agregado");
            return;
        }

        var result = await operations.ExportDiagnosticSummaryAsync(
            cancellationToken);
        if (!IsSuccessful(result) ||
            result.Data is not { IsCoherent: true })
        {
            CertificateSummary =
                Localizer.Text("winui.diagnostico.no_se_pudo_confirmar_un_resumen");
            return;
        }

        CertificateSummary = Localizer.Fill(
            "winui.diagnostico.certificados_detectados_con_acceso_de",
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
            TlsStatusTitle = Localizer.Text("winui.diagnostico.comprobacion_tls_cancelada");
            TlsStatusMessage =
                Localizer.Text("winui.diagnostico.no_se_modifico_la_confianza_ni_el");
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
            TlsStatusTitle = Localizer.Text("winui.diagnostico.confianza_tls_local_instalada");
            TlsStatusMessage =
                Localizer.Text("winui.diagnostico.el_motor_confirmo_la_ca_local_gestionada");
            TlsStatusSeverity = InfoBarSeverity.Success;
            HasTlsStatus = true;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            TlsStatusTitle = Localizer.Text("winui.diagnostico.instalacion_tls_cancelada");
            TlsStatusMessage =
                Localizer.Text("winui.diagnostico.no_se_recibio_confirmacion_de_que_la");
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
            TlsStatusTitle = Localizer.Text("winui.diagnostico.confianza_tls_local_retirada");
            TlsStatusMessage = Localizer.Fill(
                "winui.diagnostico.el_motor_confirmo_la_retirada_de_la",
                ("count", result.Data.ToString()));
            TlsStatusSeverity = InfoBarSeverity.Success;
            HasTlsStatus = true;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            TlsStatusTitle = Localizer.Text("winui.diagnostico.retirada_tls_cancelada");
            TlsStatusMessage =
                Localizer.Text("winui.diagnostico.no_se_recibio_confirmacion_de_que_la_2");
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
                TlsStatusTitle = Localizer.Text("winui.diagnostico.material_tls_local_disponible");
                TlsStatusMessage = Localizer.Fill(
                    "winui.diagnostico.se_inventariaron_artefacto_s_certificado",
                    ("artifacts", store.ArtifactCount.ToString()),
                    ("certificates", store.CertificateCount.ToString()),
                    ("keys", store.KeyCount.ToString()));
                TlsStatusSeverity = InfoBarSeverity.Success;
                break;
            case "empty":
                TlsStatusTitle = Localizer.Text("winui.diagnostico.almacen_tls_local_vacio");
                TlsStatusMessage =
                    Localizer.Text("winui.diagnostico.no_hay_material_tls_gestionado_instale");
                TlsStatusSeverity = InfoBarSeverity.Warning;
                break;
            case "not_created":
                TlsStatusTitle = Localizer.Text("winui.diagnostico.material_tls_no_creado");
                TlsStatusMessage =
                    Localizer.Text("winui.diagnostico.la_integracion_local_con_navegador");
                TlsStatusSeverity = InfoBarSeverity.Informational;
                break;
            default:
                TlsStatusTitle = Localizer.Text("winui.diagnostico.almacen_tls_no_disponible");
                TlsStatusMessage =
                    Localizer.Text("winui.diagnostico.el_motor_no_pudo_inspeccionar_de_forma");
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
        TlsStatusTitle = Localizer.Text("winui.diagnostico.no_se_pudo_completar_la_operacion_tls");
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
            ? Localizer.Text("winui.diagnostico.diagnostico_cancelado")
            : observedSteps.Any(step =>
                step.Status == DiagnosticStepStatus.Failure)
                ? Localizer.Text("winui.diagnostico.se_detecto_un_problema")
                : Localizer.Text("winui.diagnostico.comprobaciones_terminadas");
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
                Localizer.Text("winui.diagnostico.el_diagnostico_se_cancelo_las_fases");
            responsibility =
                Localizer.Text("winui.diagnostico.la_cancelacion_no_se_atribuye_a_ningun");
            action =
                Localizer.Text("winui.diagnostico.ejecute_de_nuevo_el_diagnostico_si_desea");
        }
        else if (failedStep is null)
        {
            var remoteClock = observedSteps.FirstOrDefault(step =>
                step.Code == RemoteClockCode);
            cause = remoteClock?.Status ==
                DiagnosticStepStatus.Success
                    ? Localizer.Text("winui.diagnostico.las_comprobaciones_disponibles_han")
                    : Localizer.Text("winui.diagnostico.las_comprobaciones_disponibles_han_2");
            responsibility =
                Localizer.Text("winui.diagnostico.no_se_ha_observado_un_fallo_atribuible");
            action =
                Localizer.Text("winui.diagnostico.si_una_firma_real_falla_abra_el");
        }
        else
        {
            (cause, responsibility, action) = owner switch
            {
                "app_local" => (
                    Localizer.Text("winui.diagnostico.se_ha_detectado_un_problema_en_este"),
                    Localizer.Text("winui.diagnostico.el_fallo_observado_pertenece_a_una_fase"),
                    failedStep.SuggestedAction ??
                        Localizer.Text("winui.diagnostico.revise_la_fase_marcada_con_una_y_vuelva")),
                "remote_service" => (
                    Localizer.Text("winui.diagnostico.la_evidencia_recibida_situa_el_problema"),
                    Localizer.Text("winui.diagnostico.el_fallo_observado_no_pertenece_al_canal"),
                    failedStep.SuggestedAction ??
                        Localizer.Text("winui.diagnostico.conserve_el_diagnostico_y_contacte_con")),
                "@firma" => (
                    Localizer.Text("winui.diagnostico.la_evidencia_recibida_situa_el_problema_2"),
                    Localizer.Text("winui.diagnostico.el_fallo_observado_ha_sido_atribuido_por"),
                    failedStep.SuggestedAction ??
                        Localizer.Text("winui.diagnostico.conserve_el_diagnostico_y_contacte_con_2")),
                _ => (
                    Localizer.Text("winui.diagnostico.se_ha_detectado_un_fallo_pero_su_origen"),
                    Localizer.Text("winui.diagnostico.la_evidencia_disponible_no_permite"),
                    failedStep.SuggestedAction ??
                        Localizer.Text("winui.diagnostico.conserve_el_diagnostico_y_solicite")),
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
                Localizer.Text("winui.diagnostico.el_asistente_ejecuto_unicamente_acciones"),
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
                Localizer.Text("winui.diagnostico.el_motor_respondio_pero_el_resumen"),
            LikelyOwner = owner,
            ResponsibilityMessage =
                Localizer.Text("winui.diagnostico.el_resultado_incoherente_se_ha_observado"),
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
                    Localizer.Text("winui.diagnostico.la_fase_no_se_ejecuto_porque_el"),
                    Localizer.Text("winui.diagnostico.ejecute_de_nuevo_el_diagnostico_para")));
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
            Localizer.Text("winui.diagnostico.canal_seguro_con_el_motor_local"),
            "app_local",
            Localizer.Text("winui.diagnostico.se_comprobara_al_iniciar_el_asistente")),
        UnknownStep(
            EnginePingCode,
            Localizer.Text("winui.diagnostico.respuesta_del_motor_local"),
            "app_local",
            Localizer.Text("winui.diagnostico.pendiente_de_una_peticion_ping_real")),
        UnknownStep(
            LocalClockCode,
            Localizer.Text("winui.diagnostico.reloj_local"),
            "app_local",
            Localizer.Text("winui.diagnostico.pendiente_de_una_lectura_real_y_del")),
        UnknownStep(
            CertificatesCode,
            Localizer.Text("winui.diagnostico.certificados_utilizables"),
            "certificate_store",
            Localizer.Text("winui.diagnostico.pendiente_del_resumen_local_de")),
        UnknownStep(
            CertificateAccessCode,
            Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
            "app_local",
            Localizer.Text("winui.diagnostico.pendiente_del_inventario_de_gestores_e")),
        UnknownStep(
            ProxyStoreCode,
            Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
            "network_proxy",
            Localizer.Text("winui.diagnostico.pendiente_del_inventario_seguro_del")),
        UnknownStep(
            TlsStoreCode,
            Localizer.Text("winui.diagnostico.material_tls_local"),
            "app_local",
            Localizer.Text("winui.diagnostico.pendiente_del_inventario_tls_gestionado")),
        UnknownStep(
            RemoteClockCode,
            Localizer.Text("winui.diagnostico.hora_del_servidor_remoto_observado"),
            "remote_service",
            Localizer.Text("winui.diagnostico.no_comprobado_todavia_no_hay_un_origen")),
        UnknownStep(
            GovernmentAFirmaCode,
            Localizer.Text("winui.diagnostico.plataforma_firma"),
            "@firma",
            Localizer.Text("winui.diagnostico.no_comprobado_ninguna_accion_publicada")),
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
            Localizer.Text("winui.diagnostico.abra_el_diagnostico_de_una_operacion"));

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
            Label = SafeIpcText.Clean(label, 160, Localizer.Text("winui.diagnostico.paso_observado")),
            Status = status,
            Owner = owner,
            UserMessage = SafeIpcText.Clean(
                message,
                512,
                Localizer.Text("winui.diagnostico.sin_detalle_adicional")),
            SuggestedAction = SafeIpcText.Clean(
                action,
                512,
                Localizer.Text("winui.diagnostico.no_hay_una_accion_especifica_registrada")),
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
        LocalChannelCode => Localizer.Text("winui.diagnostico.canal_seguro_con_el_motor_local"),
        EnginePingCode => Localizer.Text("winui.diagnostico.respuesta_del_motor_local"),
        LocalClockCode => Localizer.Text("winui.diagnostico.diagnostico_de_fecha_y_hora"),
        CertificatesCode => Localizer.Text("winui.diagnostico.certificados_utilizables"),
        CertificateAccessCode => Localizer.Text("winui.diagnostico.acceso_local_a_certificados"),
        ProxyStoreCode => Localizer.Text("winui.diagnostico.almacen_seguro_del_proxy"),
        TlsStoreCode => Localizer.Text("winui.diagnostico.material_tls_local"),
        _ => Localizer.Text("winui.diagnostico.comprobacion_local"),
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
            Localizer.Text("winui.diagnostico.motor_local_conectado_el_asistente"));
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
