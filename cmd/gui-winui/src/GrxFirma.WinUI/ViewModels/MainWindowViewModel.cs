// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Services;

namespace GrxFirma.WinUI.ViewModels;

public sealed class MainWindowViewModel : ObservableObject
{
    private string _connectionStatus = Localizer.Text("winui.ventana.motor_local_no_conectado");
    private string _activePageTitle = Localizer.Text("winui.comun.firmar");
    private bool _isBackendReady;
    private bool _hasConnectionError;
    private bool _isConnectionNoticeOpen = true;
    private bool _isUpdateNoticeOpen;
    private string _updateNoticeMessage = string.Empty;
    private OperationDiagnostic? _currentDiagnostic;

    public string ConnectionStatus
    {
        get => _connectionStatus;
        private set => SetProperty(ref _connectionStatus, Localizer.Text(value));
    }

    public string ActivePageTitle
    {
        get => _activePageTitle;
        set => SetProperty(ref _activePageTitle, Localizer.Text(value));
    }

    public bool IsBackendReady
    {
        get => _isBackendReady;
        private set => SetProperty(ref _isBackendReady, value);
    }

    public bool HasConnectionError
    {
        get => _hasConnectionError;
        private set => SetProperty(ref _hasConnectionError, value);
    }

    public bool IsConnectionNoticeOpen
    {
        get => _isConnectionNoticeOpen;
        private set => SetProperty(ref _isConnectionNoticeOpen, value);
    }

    public OperationDiagnostic? CurrentDiagnostic
    {
        get => _currentDiagnostic;
        private set => SetProperty(ref _currentDiagnostic, value);
    }

    public bool IsUpdateNoticeOpen
    {
        get => _isUpdateNoticeOpen;
        private set => SetProperty(ref _isUpdateNoticeOpen, value);
    }

    public string UpdateNoticeMessage
    {
        get => _updateNoticeMessage;
        private set => SetProperty(ref _updateNoticeMessage, Localizer.Text(value));
    }

    public void SetUpdateAvailable(string message)
    {
        UpdateNoticeMessage = message;
        IsUpdateNoticeOpen = true;
    }

    public void HideUpdateNotice() => IsUpdateNoticeOpen = false;

    public void SetStandalone()
    {
        IsBackendReady = false;
        HasConnectionError = false;
        IsConnectionNoticeOpen = true;
        CurrentDiagnostic = null;
        ConnectionStatus =
            Localizer.Text("winui.ventana.interfaz_iniciada_sin_motor_local_las");
    }

    public void SetConnecting()
    {
        IsBackendReady = false;
        HasConnectionError = false;
        IsConnectionNoticeOpen = true;
        CurrentDiagnostic = null;
        ConnectionStatus = Localizer.Text("winui.ventana.comprobando_la_identidad_del_motor_local");
    }

    public void SetConnected(IpcHello? hello)
    {
        IsBackendReady = hello is not null;
        HasConnectionError = hello is null;
        // El estado correcto no necesita una banda permanente en cada pantalla.
        // La conexión pendiente y los fallos siguen mostrando sus avisos.
        IsConnectionNoticeOpen = false;
        CurrentDiagnostic = hello is null
            ? BuildConnectionDiagnostic(
                Localizer.Text("winui.ventana.el_motor_local_no_confirmo_el_protocolo"),
                "unsupported_protocol",
                "protocol",
                "app_local")
            : null;
        ConnectionStatus = hello is null
            ? Localizer.Text("winui.ventana.el_motor_local_no_confirmo_el_protocolo")
            : Localizer.Fill("winui.ventana.motor_local_conectado_mediante",
                ("protocol", DesktopIpcProtocol.Name));
    }

    public void SetConnectionFailure(
        string safeMessage,
        string failureCode = "connection_failed",
        string phase = "admission",
        string likelyOwner = "unknown")
    {
        IsBackendReady = false;
        HasConnectionError = true;
        IsConnectionNoticeOpen = false;
        ConnectionStatus = SafeIpcText.Clean(
            safeMessage,
            512,
            Localizer.Text("winui.ventana.no_se_pudo_conectar_con_el_motor_local"));
        CurrentDiagnostic = BuildConnectionDiagnostic(
            ConnectionStatus,
            failureCode,
            phase,
            likelyOwner);
    }

    private static OperationDiagnostic BuildConnectionDiagnostic(
        string message,
        string failureCode,
        string phase,
        string likelyOwner)
    {
        var safeCode = SafeIpcText.Clean(
            failureCode,
            64,
            "connection_failed");
        var safePhase = phase == "protocol" ? "protocol" : "admission";
        var safeOwner = likelyOwner switch
        {
            "app_local" => "app_local",
            "environment" => "environment",
            _ => "unknown",
        };
        var protocolFailure = safePhase == "protocol";
        var ownerKnown = safeOwner != "unknown";
        return new OperationDiagnostic
        {
            Category = safeOwner,
            FailureCode = safeCode,
            UserMessage = message,
            ExpertMessage = protocolFailure
                ? Localizer.Text("winui.ventana.la_negociacion_del_protocolo_local")
                : Localizer.Text("winui.ventana.la_admision_del_canal_local_termino"),
            LikelyOwner = safeOwner,
            ResponsibilityMessage = ownerKnown
                ? Localizer.Text("winui.ventana.el_fallo_se_ha_localizado_en_esta")
                : Localizer.Text("winui.ventana.el_canal_local_fallo_pero_con_la"),
            SuggestedAction = protocolFailure
                ? Localizer.Text("winui.ventana.compruebe_que_la_interfaz_y_el_motor")
                : Localizer.Text("winui.ventana.cierre_la_aplicacion_y_vuelva_a_abrirla"),
            UserCanResolveDirectly = ownerKnown,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = safeCode,
                    Label = protocolFailure
                        ? Localizer.Text("winui.ventana.negociacion_del_protocolo_local")
                        : Localizer.Text("winui.ventana.admision_del_canal_local"),
                    Status = DiagnosticStepStatus.Failure,
                    Owner = safeOwner,
                    UserMessage = message,
                    SuggestedAction = protocolFailure
                        ? Localizer.Text("winui.ventana.actualice_o_repare_conjuntamente_la")
                        : Localizer.Text("winui.ventana.vuelva_a_iniciar_la_aplicacion_desde_el"),
                    EvidenceRef = $"phase:{safePhase}",
                },
            ],
        };
    }
}
