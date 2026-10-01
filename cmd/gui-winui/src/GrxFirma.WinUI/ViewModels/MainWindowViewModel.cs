// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Diagnostics;

namespace GrxFirma.WinUI.ViewModels;

public sealed class MainWindowViewModel : ObservableObject
{
    private string _connectionStatus = "Motor local no conectado.";
    private string _activePageTitle = "Firmar";
    private bool _isBackendReady;
    private bool _hasConnectionError;
    private bool _isConnectionNoticeOpen = true;
    private bool _isUpdateNoticeOpen;
    private string _updateNoticeMessage = string.Empty;
    private OperationDiagnostic? _currentDiagnostic;

    public string ConnectionStatus
    {
        get => _connectionStatus;
        private set => SetProperty(ref _connectionStatus, value);
    }

    public string ActivePageTitle
    {
        get => _activePageTitle;
        set => SetProperty(ref _activePageTitle, value);
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
        private set => SetProperty(ref _updateNoticeMessage, value);
    }

    public void SetUpdateAvailable(
        string latestVersion,
        string currentVersion)
    {
        var latest = CleanVersion(latestVersion);
        var current = CleanVersion(currentVersion);
        UpdateNoticeMessage =
            $"Está disponible GrxFirma {latest}; esta instalación usa {current}. Revise las notas oficiales. No se descargará ni ejecutará nada automáticamente.";
        IsUpdateNoticeOpen = true;
    }

    private static string CleanVersion(string? value)
    {
        var cleaned = new string(
            SafeIpcText.Clean(value, 64, "desconocida")
                .Where(character =>
                    char.IsAsciiLetterOrDigit(character) ||
                    character is '.' or '-' or '+' or '_')
                .ToArray());
        return cleaned.Length == 0
            ? "desconocida"
            : cleaned;
    }

    public void SetStandalone()
    {
        IsBackendReady = false;
        HasConnectionError = false;
        IsConnectionNoticeOpen = true;
        CurrentDiagnostic = null;
        ConnectionStatus =
            "Interfaz iniciada sin motor local. Las operaciones permanecen deshabilitadas.";
    }

    public void SetConnecting()
    {
        IsBackendReady = false;
        HasConnectionError = false;
        IsConnectionNoticeOpen = true;
        CurrentDiagnostic = null;
        ConnectionStatus = "Comprobando la identidad del motor local…";
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
                "El motor local no confirmó el protocolo esperado.",
                "unsupported_protocol",
                "protocol",
                "app_local")
            : null;
        ConnectionStatus = hello is null
            ? "El motor local no confirmó el protocolo esperado."
            : $"Motor local conectado mediante {DesktopIpcProtocol.Name}.";
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
            "No se pudo conectar con el motor local.");
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
                ? "La negociación del protocolo local devolvió datos incompatibles. No se ha repetido ninguna acción."
                : "La admisión del canal local terminó antes de habilitar operaciones. No se ha repetido ninguna acción.",
            LikelyOwner = safeOwner,
            ResponsibilityMessage = ownerKnown
                ? "El fallo se ha localizado en esta instalación o en el entorno local del equipo."
                : "El canal local falló, pero con la evidencia disponible no se puede atribuir el responsable.",
            SuggestedAction = protocolFailure
                ? "Compruebe que la interfaz y el motor pertenecen a la misma versión instalada. Si se repite, conserve el código y solicite soporte."
                : "Cierre la aplicación y vuelva a abrirla desde el lanzador. Si se repite, conserve el código y solicite soporte.",
            UserCanResolveDirectly = ownerKnown,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = safeCode,
                    Label = protocolFailure
                        ? "Negociación del protocolo local"
                        : "Admisión del canal local",
                    Status = DiagnosticStepStatus.Failure,
                    Owner = safeOwner,
                    UserMessage = message,
                    SuggestedAction = protocolFailure
                        ? "Actualice o repare conjuntamente la interfaz y el motor local."
                        : "Vuelva a iniciar la aplicación desde el lanzador.",
                    EvidenceRef = $"phase:{safePhase}",
                },
            ],
        };
    }
}
